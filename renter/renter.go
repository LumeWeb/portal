package renter

import (
	"context"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/renterd/api"
	busClient "go.sia.tech/renterd/bus/client"
	"go.sia.tech/renterd/object"
	workerClient "go.sia.tech/renterd/worker/client"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"io"
	"math"
	"net/url"
	"strconv"
)

type ReaderFactory func(start uint, end uint) (io.ReadCloser, error)
type UploadIDHandler func(uploadID string)

type RenterServiceParams struct {
	fx.In
	Config *viper.Viper
	Logger *zap.Logger
	Cron   *cron.CronServiceDefault
}

type RenterDefault struct {
	busClient    *busClient.Client
	workerClient *workerClient.Client
	config       *viper.Viper
	logger       *zap.Logger
	cron         *cron.CronServiceDefault
}

type MultiPartUploadParams struct {
	ReaderFactory    ReaderFactory
	Bucket           string
	FileName         string
	Size             uint64
	ExistingUploadID string
	UploadIDHandler  UploadIDHandler
}

var Module = fx.Module("renter",
	fx.Options(
		fx.Provide(NewRenterService),
		fx.Invoke(func(r *RenterDefault) error {
			return r.init()
		}),
	),
)

func NewRenterService(params RenterServiceParams) *RenterDefault {
	return &RenterDefault{
		config: params.Config,
		logger: params.Logger,
		cron:   params.Cron,
	}
}

func (r *RenterDefault) CreateBucketIfNotExists(bucket string) error {
	_, err := r.busClient.Bucket(context.Background(), bucket)

	if err == nil {
		return nil
	}

	if err != nil {
		if !errors.Is(err, api.ErrBucketNotFound) {
			return err
		}
	}

	err = r.busClient.CreateBucket(context.Background(), bucket, api.CreateBucketOptions{
		Policy: api.BucketPolicy{
			PublicReadAccess: false,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) UploadObject(ctx context.Context, file io.Reader, bucket string, hash string) error {
	_, err := r.workerClient.UploadObject(ctx, file, bucket, hash, api.UploadObjectOptions{})

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) init() error {
	addr := r.config.GetString("core.sia.url")
	passwd := r.config.GetString("core.sia.key")

	addrURL, err := url.Parse(addr)

	if err != nil {
		return err
	}

	addrURL.Path = "/api/worker"

	r.workerClient = workerClient.New(addrURL.String(), passwd)

	addrURL.Path = "/api/bus"

	r.busClient = busClient.New(addrURL.String(), passwd)

	return nil
}

func (r *RenterDefault) GetObject(ctx context.Context, protocol string, hash string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error) {
	return r.workerClient.GetObject(ctx, protocol, hash, options)
}

func (r *RenterDefault) GetSetting(ctx context.Context, setting string, out any) error {
	err := r.busClient.Setting(ctx, setting, out)

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) MultipartUpload(params MultiPartUploadParams) error {
	size := params.Size
	rf := params.ReaderFactory
	bucket := params.Bucket
	fileName := params.FileName
	ctx := context.Background()
	idHandler := params.UploadIDHandler

	var redundancy api.RedundancySettings

	err := r.GetSetting(ctx, "redundancy", &redundancy)
	if err != nil {
		return err
	}

	slabSize := uint64(redundancy.MinShards * rhpv2.SectorSize)
	parts := uint64(math.Ceil(float64(size) / float64(slabSize)))
	uploadParts := make([]api.MultipartCompletedPart, parts)

	upload, err := r.busClient.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{Key: object.NoOpKey})
	if err != nil {
		return err
	}

	if idHandler != nil {
		idHandler(upload.UploadID)
	}

	for i := uint64(0); i < parts; i++ {
		start := i * slabSize

		end := start + slabSize
		if end > size {
			end = size
		}
		nextChan := make(chan string, 0)
		errChan := make(chan error, 0)

		partNumber := int(i + 1)

		job := r.cron.RetryableTask(cron.RetryableTaskParams{
			Name: fileName + "-part-" + strconv.FormatUint(i, 10),
			Function: func() error {
				reader, err := rf(uint(start), uint(end))
				defer func(reader io.ReadCloser) {
					err := reader.Close()
					if err != nil {
						r.logger.Error("failed to close reader", zap.Error(err))
					}
				}(reader)

				if err != nil {
					return err
				}

				ret, err := r.workerClient.UploadMultipartUploadPart(context.Background(), reader, bucket, fileName, upload.UploadID, partNumber, api.UploadMultipartUploadPartOptions{})
				if err != nil {
					return err
				}

				nextChan <- ret.ETag
				return nil
			},
			Limit: 10,
			Error: func(jobID uuid.UUID, jobName string, err error) {
				if errors.Is(err, cron.ErrRetryLimitReached) {
					r.logger.Error("failed to upload part", zap.String("jobName", jobName), zap.Error(err))
					errChan <- err
				}
			},
		})

		_, err = r.cron.CreateJob(job)
		if err != nil {
			r.logger.Error("failed to create job", zap.Error(err))
			return err
		}

		uploadParts[i] = api.MultipartCompletedPart{
			PartNumber: partNumber,
		}

		select {
		case err = <-errChan:
			return fmt.Errorf("failed to upload part %d: %s", i, err.Error())
		case etag := <-nextChan:
			uploadParts[i].ETag = etag
		}

	}

	_, err = r.busClient.CompleteMultipartUpload(ctx, bucket, fileName, upload.UploadID, uploadParts)
	if err != nil {
		return err
	}

	return nil
}
