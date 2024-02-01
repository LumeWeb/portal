package renter

import (
	"context"
	"errors"
	"github.com/spf13/viper"
	"go.sia.tech/renterd/api"
	busClient "go.sia.tech/renterd/bus/client"
	workerClient "go.sia.tech/renterd/worker/client"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"io"
	"net/url"
)

type RenterServiceParams struct {
	fx.In
	Config *viper.Viper
	Logger *zap.Logger
}

type RenterDefault struct {
	busClient    *busClient.Client
	workerClient *workerClient.Client
	config       *viper.Viper
	logger       *zap.Logger
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
