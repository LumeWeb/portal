package core

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	mh "github.com/multiformats/go-multihash"
	"io"
	"time"
)

type StorageUploadStatus string

const STORAGE_SERVICE = "storage"

const (
	StorageUploadStatusUnknown    StorageUploadStatus = "unknown"
	StorageUploadStatusProcessing StorageUploadStatus = "processing"
	StorageUploadStatusActive     StorageUploadStatus = "completed"
)

var (
	ErrProofNotSupported = errors.New("protocol does not support proofs")
)

type FileNameEncoderFunc func([]byte) string

type StorageHash interface {
	Proof() []byte
	Multihash() mh.Multihash
	ProofExists() bool
	Type() uint64
}

type StorageProtocol interface {
	Name() string
	EncodeFileName(StorageHash) string
	Hash(r io.Reader, size uint64) (StorageHash, error)
}

type StorageUploadRequest interface {
	Protocol() StorageProtocol
	SetProtocol(StorageProtocol)
	Data() io.ReadSeeker
	SetData(io.ReadSeeker)
	Size() uint64
	SetSize(uint64)
	MuParams() *MultipartUploadParams
	SetMuParams(*MultipartUploadParams)
	Hash() StorageHash
	SetHash(StorageHash)
}

// StorageUploadOption defines a function to configure StorageUploadRequest
type StorageUploadOption func(StorageUploadRequest)

// WithProtocol sets the protocol for the upload request
func StorageUploadWithProtocol(protocol StorageProtocol) StorageUploadOption {
	return func(r StorageUploadRequest) {
		r.SetProtocol(protocol)
	}
}

// WithData sets the data for the upload request
func StorageUploadWithData(data io.ReadSeeker) StorageUploadOption {
	return func(r StorageUploadRequest) {
		r.SetData(data)
	}
}

// WithSize sets the size for the upload request
func StorageUploadWithSize(size uint64) StorageUploadOption {
	return func(r StorageUploadRequest) {
		r.SetSize(size)
	}
}

// WithMultipartUploadParams sets the multipart upload parameters for the upload request
func StorageUploadWithMultipartUploadParams(params *MultipartUploadParams) StorageUploadOption {
	return func(r StorageUploadRequest) {
		r.SetMuParams(params)
	}
}

// WithProof sets the proof for the upload request
func StorageUploadWithProof(proof StorageHash) StorageUploadOption {
	return func(r StorageUploadRequest) {
		r.SetHash(proof)
	}
}

type StorageService interface {
	UploadObject(ctx context.Context, request StorageUploadRequest) (*UploadMetadata, error)
	UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof StorageHash, size uint64) error
	DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash StorageHash, start int64) (io.ReadCloser, error)
	DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) error
	DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) error
	S3Client(ctx context.Context) (*s3.Client, error)
	S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error
	UploadStatus(ctx context.Context, protocol StorageProtocol, objectName string) (StorageUploadStatus, *time.Time, error)

	Service
}
