package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/renterd/api"
	autoPilotClient "go.sia.tech/renterd/autopilot"
	busClient "go.sia.tech/renterd/bus/client"
	"go.sia.tech/renterd/object"
	workerClient "go.sia.tech/renterd/worker/client"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"math"
	"net/url"
	"strings"
)

var _ core.RenterService = (*RenterDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.RENTER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewRenterService()
		},
	})
}

type RenterDefault struct {
	busClient       *busClient.Client
	workerClient    *workerClient.Client
	autoPilotClient *autoPilotClient.Client
	ctx             core.Context
	config          config.Manager
	db              *gorm.DB
	logger          *core.Logger
}

func NewRenterService() (*RenterDefault, []core.ContextBuilderOption, error) {
	renter := &RenterDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			renter.ctx = ctx
			renter.config = ctx.Config()
			renter.db = ctx.DB()
			renter.logger = ctx.Logger()
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			err := renter.init()
			if err != nil {
				renter.logger.Error("failed to initialize renter service", zap.Error(err))
			}

			return err
		}),
	)

	return renter, opts, nil
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

func (r *RenterDefault) ImportObjectMetadata(ctx context.Context, bucket string, fileName string, object_ object.Object) error {
	cfg, err := r.autoPilotClient.Config()
	if err != nil {
		return err
	}
	return r.busClient.AddObject(ctx, bucket, fileName, cfg.Contracts.Set, object_, api.AddObjectOptions{})
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

	addrURL.Path = "/api/autopilot"

	r.autoPilotClient = autoPilotClient.NewClient(addrURL.String(), passwd)

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

func (r *RenterDefault) DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error {
	return r.busClient.DeleteObject(ctx, bucket, fileName, api.DeleteObjectOptions{})
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

func (r *RenterDefault) UploadObjectMultipart(ctx context.Context, params *core.MultiPartUploadParams) error {
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
	uploadParts := make([]api.MultipartCompletedPart, 0)

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
		upload, err := r.busClient.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{GenerateKey: true})
		if err != nil {
			return err
		}

		uploadId = upload.UploadID
		siaUpload.UploadID = uploadId
		if tx := r.db.WithContext(ctx).Model(&siaUpload).Save(&siaUpload); tx.Error != nil {
			return tx.Error
		}
	} else {
		existing, err := r.busClient.MultipartUploadParts(ctx, bucket, fileName, uploadId, 0, 0)

		if err != nil {
			uploadId = ""
		} else {
			for _, part := range existing.Parts {
				if uint64(part.Size) != slabSize {
					break
				}
				partNumber := part.PartNumber
				uploadParts = append(uploadParts, api.MultipartCompletedPart{
					PartNumber: partNumber,
					ETag:       part.ETag,
				})
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
		offset := int(i * slabSize)

		opts := api.UploadMultipartUploadPartOptions{}
		opts.EncryptionOffset = &offset

		ret, err := r.workerClient.UploadMultipartUploadPart(ctx, lr, bucket, fileName, uploadId, partNumber, opts)
		if err != nil {
			return err
		}

		uploadParts = append(uploadParts, api.MultipartCompletedPart{
			PartNumber: partNumber,
			ETag:       ret.ETag,
		})
	}

	_, err = r.busClient.CompleteMultipartUpload(ctx, bucket, fileName, uploadId, uploadParts, api.CompleteMultipartOptions{})
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
