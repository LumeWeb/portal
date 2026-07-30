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

// SharedObject contains all the metadata necessary to retrieve and decrypt
// an object without additional account context. Each slab includes its own
// unencrypted EncryptionKey, so a consumer can fetch and decrypt sectors
// directly from the Sia network using the slab layout and sector roots.
//
// DataKey is the object-level data key used to decrypt the slab data after
// it has been reassembled. It is returned unencrypted so consumers can
// decrypt the content without access to the indexer or app key.
type SharedObject struct {
	Slabs   []SlabSlice `json:"slabs"`
	DataKey [32]byte    `json:"data_key"`
}

// SlabSlice represents a slice of a slab that is part of an object.
type SlabSlice struct {
	Version       uint8          `json:"version"`
	EncryptionKey [32]byte       `json:"encryption_key"`
	MinShards     uint           `json:"min_shards"`
	Sectors       []PinnedSector `json:"sectors"`
	Offset        uint32         `json:"offset"`
	Length        uint32         `json:"length"`
}

// PinnedSector is a sector that has been pinned to a host.
type PinnedSector struct {
	Root    [32]byte `json:"root"`
	HostKey [32]byte `json:"host_key"`
}

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
	SharedObject(ctx context.Context, bucket string, fileName string) (*SharedObject, *models.RenterObject, error)

	Service
}
