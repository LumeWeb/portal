package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/renterd/api"
	"go.sia.tech/renterd/object"
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
type RenterService interface {
	CreateBucketIfNotExists(bucket string) error
	UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string) error
	ImportObjectMetadata(ctx context.Context, bucket string, fileName string, object_ object.Object) error
	GetObject(ctx context.Context, bucket string, fileName string, options api.DownloadObjectOptions) (*api.GetObjectResponse, error)
	GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*api.Object, error)
	DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error
	GetSetting(ctx context.Context, setting string, out any) error
	UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.SiaUpload, error)
	UploadObjectMultipart(ctx context.Context, params *MultipartUploadParams) error
	DeleteObject(ctx context.Context, bucket string, fileName string) error
	UpdateGougingSettings(ctx context.Context, settings api.GougingSettings) error
	GougingSettings(ctx context.Context) (api.GougingSettings, error)
	RedundancySettings(ctx context.Context) (api.RedundancySettings, error)
	SlabSize(ctx context.Context) (uint64, error)

	Service
}
