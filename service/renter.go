package service

import (
	"context"
	"errors"
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	renterInternal "go.lumeweb.com/portal/service/internal/renter"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/v2/api"
	autoPilotClient "go.sia.tech/renterd/v2/autopilot"
	busClient "go.sia.tech/renterd/v2/bus/client"
	workerClient "go.sia.tech/renterd/v2/worker/client"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"math"
	"net/url"
	"strings"
	"time"
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
	clientManager   *renterInternal.ClientManager
}

func NewRenterService() (*RenterDefault, []core.ContextBuilderOption, error) {
	renter := &RenterDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			renter.ctx = ctx
			renter.config = ctx.Config()
			renter.db = ctx.DB()
			renter.logger = ctx.ServiceLogger(renter)
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			err := renter.init()
			if err != nil {
				return fmt.Errorf("failed to initialize renter service: %w", err)
			}

			if err != nil {
				return err
			}

			return nil
		}),
	)

	return renter, opts, nil
}

func (r *RenterDefault) ID() string {
	return core.RENTER_SERVICE
}

func (r *RenterDefault) CreateBucketIfNotExists(bucket string) error {
	client, err := r.getBusClient()
	if err != nil {
		return err
	}

	_, err = client.Bucket(context.Background(), bucket)
	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), api.ErrBucketNotFound.Error()) {
		return err
	}

	err = client.CreateBucket(context.Background(), bucket, api.CreateBucketOptions{
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
	client, err := r.getWorkerClient()
	if err != nil {
		return err
	}

	fileName = "/" + strings.TrimLeft(fileName, "/")
	_, err = client.UploadObject(ctx, file, bucket, fileName, api.UploadObjectOptions{})

	if err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) init() error {
	r.clientManager = renterInternal.NewClientManager(r.ctx)
	if err := r.clientManager.Start(); err != nil {
		return fmt.Errorf("failed to start client manager: %w", err)
	}

	clusterEnabled := r.ctx.Config().Config().Core.ClusterEnabled() && r.ctx.Config().Config().Core.Storage.Sia.Cluster

	if !clusterEnabled {
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

		_, stateErr := r.busClient.State(r.ctx)
		if stateErr != nil {
			return fmt.Errorf("renter status check: failed to get renter state: %w", stateErr)
		}
	}

	return nil
}

func (r *RenterDefault) getBusClient() (*busClient.Client, error) {
	if !r.ctx.Config().Config().Core.ClusterEnabled() || !r.ctx.Config().Config().Core.Storage.Sia.Cluster {
		if r.busClient == nil {
			return nil, fmt.Errorf("bus client not initialized")
		}
		return r.busClient, nil
	}

	node, err := r.clientManager.GetNextNode(renterInternal.ClientTypeBus)
	if err != nil {
		return nil, fmt.Errorf("failed to get bus node: %w", err)
	}

	client := busClient.New(node.URL, r.config.Config().Core.Storage.Sia.Key)
	return client, nil
}

func (r *RenterDefault) getWorkerClient() (*workerClient.Client, error) {
	if !r.ctx.Config().Config().Core.ClusterEnabled() || !r.ctx.Config().Config().Core.Storage.Sia.Cluster {
		if r.workerClient == nil {
			return nil, fmt.Errorf("worker client not initialized")
		}
		return r.workerClient, nil
	}

	node, err := r.clientManager.GetNextNode(renterInternal.ClientTypeWorker)
	if err != nil {
		return nil, fmt.Errorf("failed to get worker node: %w", err)
	}

	client := workerClient.New(node.URL, r.config.Config().Core.Storage.Sia.Key)
	return client, nil
}

func (r *RenterDefault) GetObject(ctx context.Context, bucket string, fileName string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error) {
	client, err := r.getWorkerClient()
	if err != nil {
		return nil, err
	}
	fileName = "/" + strings.TrimLeft(fileName, "/")
	return client.GetObject(ctx, bucket, fileName, options)
}

func (r *RenterDefault) GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*api.Object, error) {
	client, err := r.getBusClient()
	if err != nil {
		return nil, err
	}
	ret, err := client.Object(ctx, bucket, fileName, api.GetObjectOptions{})

	if err != nil {
		return nil, err
	}

	return &ret, nil
}

func (r *RenterDefault) DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error {
	client, err := r.getBusClient()
	if err != nil {
		return err
	}
	return client.DeleteObject(ctx, bucket, fileName)
}

func (r *RenterDefault) UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.SiaUpload, error) {
	var siaUpload models.SiaUpload

	siaUpload.Bucket = bucket
	siaUpload.Key = fileName

	if err := db.RetryOnLock(r.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.SiaUpload{}).Where(&siaUpload).First(&siaUpload)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, &siaUpload, nil
}

func (r *RenterDefault) UploadObjectMultipart(ctx context.Context, params *core.MultipartUploadParams) error {
	size := params.Size
	rf := params.ReaderFactory
	bucket := params.Bucket
	fileName := params.FileName
	fileName = "/" + strings.TrimLeft(fileName, "/")

	slabSize, err := r.SlabSize(ctx)
	if err != nil {
		return err
	}

	parts := uint64(math.Ceil(float64(size) / float64(slabSize)))
	uploadParts := make([]api.MultipartCompletedPart, 0)

	var uploadId string
	start := uint64(0)

	var siaUpload models.SiaUpload

	siaUpload.Bucket = bucket
	siaUpload.Key = fileName

	err = db.RetryOnLock(r.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&siaUpload).First(&siaUpload)

	})
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else {
		uploadId = siaUpload.UploadID
	}

	if len(uploadId) == 0 {
		client, err := r.getBusClient()
		if err != nil {
			return err
		}
		upload, err := client.CreateMultipartUpload(ctx, bucket, fileName, api.CreateMultipartOptions{})
		if err != nil {
			return err
		}

		uploadId = upload.UploadID
		siaUpload.UploadID = uploadId
		if err = db.RetryOnLock(r.db, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Create(&siaUpload)
		}); err != nil {
			return err
		}
	} else {
		client, err := r.getBusClient()
		if err != nil {
			return err
		}
		existing, err := client.MultipartUploadParts(ctx, bucket, fileName, uploadId, 0, 0)

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

		client, err := r.getWorkerClient()
		if err != nil {
			return err
		}
		ret, err := client.UploadMultipartUploadPart(ctx, lr, bucket, fileName, uploadId, partNumber, opts)
		if err != nil {
			return err
		}

		uploadParts = append(uploadParts, api.MultipartCompletedPart{
			PartNumber: partNumber,
			ETag:       ret.ETag,
		})

		siaUpload.UpdatedAt = time.Now()

		if err = db.RetryOnLock(r.db, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Model(&siaUpload).Save(&siaUpload)
		}); err != nil {
			return err
		}
	}

	client, err := r.getBusClient()
	if err != nil {
		return err
	}
	_, err = client.CompleteMultipartUpload(ctx, bucket, fileName, uploadId, uploadParts, api.CompleteMultipartOptions{})
	if err != nil {
		return err
	}

	if err = db.RetryOnLock(r.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Delete(&siaUpload)
	}); err != nil {
		return err
	}

	return nil
}

func (r *RenterDefault) DeleteObject(ctx context.Context, bucket string, fileName string) error {
	return r.workerClient.DeleteObject(ctx, bucket, fileName)
}

func (r *RenterDefault) UpdateGougingSettings(ctx context.Context, settings api.GougingSettings) error {
	return r.busClient.UpdateGougingSettings(ctx, settings)
}

func (r *RenterDefault) GougingSettings(ctx context.Context) (api.GougingSettings, error) {
	client, err := r.getBusClient()
	if err != nil {
		return api.GougingSettings{}, err
	}
	settings, err := client.GougingSettings(ctx)

	if err != nil {
		return api.GougingSettings{}, err
	}

	return settings, nil
}

func (r *RenterDefault) SlabSize(ctx context.Context) (uint64, error) {
	client, err := r.getBusClient()
	if err != nil {
		return 0, err
	}

	uploadSettings, err := client.UploadSettings(ctx)
	if err != nil {
		return 0, err
	}

	return uploadSettings.Redundancy.SlabSizeNoRedundancy(), nil
}

func (r *RenterDefault) Host(ctx context.Context, host types.PublicKey) (api.Host, error) {
	client, err := r.getBusClient()
	if err != nil {
		return api.Host{}, err
	}
	return client.Host(ctx, host)
}

func (r *RenterDefault) ConsensusState(ctx context.Context) (api.ConsensusState, error) {
	client, err := r.getBusClient()
	if err != nil {
		return api.ConsensusState{}, err
	}
	return client.ConsensusState(ctx)
}

func (r *RenterDefault) RecommendedFee(ctx context.Context) (types.Currency, error) {
	client, err := r.getBusClient()
	if err != nil {
		return types.Currency{}, err
	}
	return client.RecommendedFee(ctx)
}
