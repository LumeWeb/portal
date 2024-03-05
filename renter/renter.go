package renter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"gorm.io/gorm"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/cron"
	"github.com/google/uuid"
	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/renterd/api"
	busClient "go.sia.tech/renterd/bus/client"
	"go.sia.tech/renterd/object"
	workerClient "go.sia.tech/renterd/worker/client"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ReaderFactory func(start uint, end uint) (io.ReadCloser, error)
type UploadIDHandler func(uploadID string)

type RenterServiceParams struct {
	fx.In
	Config *config.Manager
	Logger *zap.Logger
	Cron   *cron.CronServiceDefault
	Db     *gorm.DB
}

type RenterDefault struct {
	busClient    *busClient.Client
	workerClient *workerClient.Client
	config       *config.Manager
	logger       *zap.Logger
	cron         *cron.CronServiceDefault
	db           *gorm.DB
}

type MultiPartUploadParams struct {
	ReaderFactory   ReaderFactory
	Bucket          string
	FileName        string
	Size            uint64
	UploadIDHandler UploadIDHandler
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
		db:     params.Db,
	}
}

func (r *RenterDefault) CreateBucketIfNotExists(bucket string) error {
	_, err := r.busClient.Bucket(context.Background(), bucket)

	if err == nil {
		return nil
	}

	if !errors.Is(err, api.ErrBucketNotFound) {
		return err
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

func (r *RenterDefault) UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string) error {
	fileName = "/" + strings.TrimLeft(fileName, "/")
	_, err := r.workerClient.UploadObject(ctx, file, bucket, fileName, api.UploadObjectOptions{})

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) init() error {
	addr := r.config.Config().Core.Sia.URL
	passwd := r.config.Config().Core.Sia.Key

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

func (r *RenterDefault) GetObject(ctx context.Context, bucket string, fileName string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error) {
	return r.workerClient.GetObject(ctx, bucket, fileName, options)
}

func (r *RenterDefault) GetSetting(ctx context.Context, setting string, out any) error {
	err := r.busClient.Setting(ctx, setting, out)

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) UploadObjectMultipart(ctx context.Context, params *MultiPartUploadParams) error {
	size := params.Size
	rf := params.ReaderFactory
	bucket := params.Bucket
	fileName := params.FileName
	idHandler := params.UploadIDHandler

	fileName = "/" + strings.TrimLeft(fileName, "/")

	var redundancy api.RedundancySettings

	err := r.GetSetting(ctx, "redundancy", &redundancy)
	if err != nil {
		return err
	}

	slabSize := uint64(redundancy.MinShards * rhpv2.SectorSize)
	parts := uint64(math.Ceil(float64(size) / float64(slabSize)))
	uploadParts := make([]api.MultipartCompletedPart, parts)

	var uploadId string
	start := uint64(0)

	var siaUpload models.SiaUpload

	siaUpload.Bucket = bucket
	siaUpload.Key = fileName

	ret := r.db.WithContext(ctx).Model(&siaUpload).First(&siaUpload)
	if ret.Error != nil {
		if !errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return ret.Error
		}
	} else {
		uploadId = siaUpload.UploadID
	}

	if len(uploadId) > 0 {
		// TODO: Switch to using https://github.com/SiaFoundation/renterd/pull/974 after renterd is moved to core/coreutils. We cannot update until then due to WIP work.
		existing, err := r.busClient.MultipartUploadParts(ctx, bucket, fileName, uploadId, 0, 0)

		if err != nil {
			uploadId = ""
		} else {
			for _, part := range existing.Parts {
				if uint64(part.Size) != slabSize {
					break
				}
				partNumber := part.PartNumber
				uploadParts[partNumber-1] = api.MultipartCompletedPart{
					PartNumber: part.PartNumber,
					ETag:       part.ETag,
				}
			}

			if len(uploadParts) > 0 {
				start = uint64(len(uploadParts)) - 1
			}
		}

	}

	if uploadId == "" {
		upload, err := r.busClient.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{Key: object.NoOpKey})
		if err != nil {
			return err
		}

		uploadId = upload.UploadID
		if tx := r.db.WithContext(ctx).Model(&siaUpload).Save(&siaUpload); tx.Error != nil {
			return tx.Error
		}
	}

	if idHandler != nil {
		idHandler(uploadId)
	}

	for i := start; i < parts; i++ {
		start := i * slabSize

		end := start + slabSize
		if end > size {
			end = size
		}
		nextChan := make(chan string, 0)
		errChan := make(chan error, 0)

		partNumber := int(i + 1)

		job := r.cron.RetryableJob(cron.RetryableJobParams{
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

				ret, err := r.workerClient.UploadMultipartUploadPart(context.Background(), reader, bucket, fileName, uploadId, partNumber, api.UploadMultipartUploadPartOptions{})
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
		case <-ctx.Done():
			return ctx.Err()
		}

	}

	_, err = r.busClient.CompleteMultipartUpload(ctx, bucket, fileName, uploadId, uploadParts)
	if err != nil {
		return err
	}

	if tx := r.db.WithContext(ctx).Delete(&siaUpload); tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (r *RenterDefault) DeleteObject(ctx context.Context, bucket string, fileName string) error {
	return r.workerClient.DeleteObject(ctx, bucket, fileName, api.DeleteObjectOptions{})
}
