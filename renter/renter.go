package renter

import (
	"context"
	"errors"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/LumeWeb/portal/db/models"

	"gorm.io/gorm"

	"github.com/LumeWeb/portal/config"

	"github.com/LumeWeb/portal/cron"
	sia "github.com/LumeWeb/siacentral-api"
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
		fx.Provide(sia.NewSiaClient),
		fx.Provide(NewPriceTracker),
		fx.Invoke(func(r *RenterDefault) error {
			return r.init()
		}),
		fx.Invoke(func(r *PriceTracker) error {
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

	if !strings.Contains(err.Error(), api.ErrBucketNotFound.Error()) {
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
	addr := r.config.Config().Core.Storage.Sia.URL
	passwd := r.config.Config().Core.Storage.Sia.Key

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
	fileName = "/" + strings.TrimLeft(fileName, "/")
	return r.workerClient.GetObject(ctx, bucket, fileName, options)
}

func (r *RenterDefault) GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*api.Object, error) {
	ret, err := r.busClient.Object(ctx, bucket, fileName, api.GetObjectOptions{})

	if err != nil {
		return nil, err
	}

	return ret.Object, nil
}

func (r *RenterDefault) GetSetting(ctx context.Context, setting string, out any) error {
	err := r.busClient.Setting(ctx, setting, out)

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) UploadExists(ctx context.Context, bucket string, fileName string) (bool, error) {
	var siaUpload models.SiaUpload

	siaUpload.Bucket = bucket
	siaUpload.Key = fileName

	ret := r.db.WithContext(ctx).Model(&siaUpload).First(&siaUpload)
	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, ret.Error
	}

	return true, nil
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

	if len(uploadId) == 0 {
		upload, err := r.busClient.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{Key: object.NoOpKey})
		if err != nil {
			return err
		}

		uploadId = upload.UploadID
		siaUpload.UploadID = uploadId
		if tx := r.db.WithContext(ctx).Model(&siaUpload).Save(&siaUpload); tx.Error != nil {
			return tx.Error
		}
	} else {
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

	if idHandler != nil {
		idHandler(uploadId)
	}

	reader, err := rf(uint(start*slabSize), uint(0))
	if err != nil {
		return err
	}

	defer func(reader io.ReadCloser) {
		err := reader.Close()
		if err != nil {
			r.logger.Error("error closing reader", zap.Error(err))
		}
	}(reader)

	for i := start; i < parts; i++ {
		lr := io.LimitReader(reader, int64(slabSize))
		partNumber := int(i + 1)

		ret, err := r.workerClient.UploadMultipartUploadPart(ctx, lr, bucket, fileName, uploadId, partNumber, api.UploadMultipartUploadPartOptions{})
		if err != nil {
			return err
		}

		uploadParts[i] = api.MultipartCompletedPart{
			PartNumber: partNumber,
			ETag:       ret.ETag,
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

func (r *RenterDefault) UpdateGougingSettings(ctx context.Context, settings api.GougingSettings) error {
	return r.busClient.UpdateSetting(ctx, api.SettingGouging, settings)
}

func (r *RenterDefault) GougingSettings(ctx context.Context) (api.GougingSettings, error) {
	var settings api.GougingSettings
	err := r.GetSetting(ctx, api.SettingGouging, &settings)

	if err != nil {
		return api.GougingSettings{}, err
	}

	return settings, nil
}

func (r *RenterDefault) RedundancySettings(ctx context.Context) (api.RedundancySettings, error) {
	var settings api.RedundancySettings
	err := r.GetSetting(ctx, api.SettingRedundancy, &settings)

	if err != nil {
		return api.RedundancySettings{}, err
	}

	return settings, nil
}
