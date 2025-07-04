package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/renterd/v2/api"
	"io"
)

const RENTER_SERVICE = "renter"

type ReaderFactory func(start uint, end uint) (io.ReadCloser, error)
type UploadIDHandler func(uploadID string)

type MultipartUploadParams struct {
	ReaderFactory ReaderFactory
	Bucket        string
	FileName      string
	Size          uint64
}

type RenterHostFilterMode string

type RenterService interface {
	CreateBucketIfNotExists(bucket string) error
	UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string) error
	GetObject(ctx context.Context, bucket string, fileName string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error)
	GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*api.Object, error)
	DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error
	UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.SiaUpload, error)
	UploadObjectMultipart(ctx context.Context, params *MultipartUploadParams) error
	DeleteObject(ctx context.Context, bucket string, fileName string) error
	UpdateGougingSettings(ctx context.Context, settings api.GougingSettings) error
	GougingSettings(ctx context.Context) (api.GougingSettings, error)
	SlabSize(ctx context.Context) (uint64, error)

	Service
}
