package core

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.lumeweb.com/portal/bao"
	"io"
)

const (
	STORAGE_SERVICE               = "storage"
	EVENT_STORAGE_OBJECT_UPLOADED = "storage.object.uploaded"
)

func init() {
	RegisterEvent(EVENT_STORAGE_OBJECT_UPLOADED, &StorageObjectUploadedEvent{})
}

type StorageUploadStatus string

const (
	StorageUploadStatusUnknown    StorageUploadStatus = "unknown"
	StorageUploadStatusProcessing StorageUploadStatus = "processing"
	StorageUploadStatusActive     StorageUploadStatus = "completed"
)

type FileNameEncoderFunc func([]byte) string

type StorageProtocol interface {
	Name() string
	EncodeFileName([]byte) string
}

type StorageService interface {
	UploadObject(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, size uint64, muParams *MultiPartUploadParams, proof *bao.Result) (*UploadMetadata, error)
	UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof *bao.Result, size uint64) error
	HashObject(ctx context.Context, data io.Reader, size uint64) (*bao.Result, error)
	DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash []byte, start int64) (io.ReadCloser, error)
	DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
	DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
	S3Client(ctx context.Context) (*s3.Client, error)
	S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error
	UploadStatus(ctx context.Context, protocol StorageProtocol, objectName string) (StorageUploadStatus, error)

	Service
}

type StorageObjectUploadedEvent struct {
	Event
	objectMetadata *UploadMetadata
}

func (e *StorageObjectUploadedEvent) ObjectMetadata() *UploadMetadata {
	return e.objectMetadata
}
