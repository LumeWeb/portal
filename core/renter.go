package core

import (
	"context"
	"io"
	"time"

	"go.lumeweb.com/portal/db/models"
)

const RENTER_SERVICE = "renter"

type ReaderFactory func(start uint, end uint) (io.ReadCloser, error)
type UploadIDHandler func(uploadID string)

type MultipartUploadParams struct {
	ReaderFactory ReaderFactory
	Bucket        string
	FileName      string
	Size          uint64
	Hash          []byte // multihash bytes for dedup lookup
}

// DownloadRange specifies a byte range for partial object downloads.
// Offset is the starting byte position (0-indexed).
// Length is the number of bytes to download. If 0, downloads from Offset to end.
type DownloadRange struct {
	Offset int64
	Length int64
}

// DownloadOptions configures object download behavior.
type DownloadOptions struct {
	Range *DownloadRange
}

// ObjectMetadata represents metadata for a stored object.
type ObjectMetadata struct {
	Bucket   string
	Key      string
	Size     int64
	ETag     string
	ModTime  time.Time
}

type RenterHostFilterMode string

type RenterService interface {
	CreateBucketIfNotExists(bucket string) error
	UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string, hash []byte) error
	GetObject(ctx context.Context, bucket string, fileName string, options DownloadOptions) (io.ReadCloser, error)
	GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*ObjectMetadata, error)
	DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error
	UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.RenterObject, error)
	UploadObjectMultipart(ctx context.Context, params *MultipartUploadParams) error
	DeleteObject(ctx context.Context, bucket string, fileName string) error
	SlabSize(ctx context.Context) (uint64, error)

	Service
}
