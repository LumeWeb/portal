package core

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/db/models"
	"io"
	"time"
)

var _ StorageHash = (*StorageHashDefault)(nil)

type StorageUploadStatus string

const (
	STORAGE_SERVICE        = "storage"
	TEMPORARY_UPLOADS_PATH = "uploads"
)

const (
	StorageUploadStatusUnknown    StorageUploadStatus = "unknown"
	StorageUploadStatusProcessing StorageUploadStatus = "processing"
	StorageUploadStatusActive     StorageUploadStatus = "completed"
)

var (
	ErrProofNotSupported = errors.New("protocol does not support proofs")
	ErrInvalidHashFormat = errors.New("could not parse hash string: not a valid multihash format")
)

type FileNameEncoderFunc func([]byte) string

type StorageHash interface {
	Proof() []byte
	Multihash() mh.Multihash
	ProofExists() bool
	CIDType() uint64
	Type() uint64
	String() string
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
	SetHashTypes(types []uint64)
	HashTypes() []uint64
	SetHashes(hashes []StorageHash)
	Hashes() []StorageHash
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
	UploadObject(ctx context.Context, request StorageUploadRequest) (*models.Upload, error)
	UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof StorageHash, size uint64) error
	DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash StorageHash, start int64) (io.ReadCloser, error)
	DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) error
	DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash StorageHash) error
	S3Client(ctx context.Context) (*s3.Client, error)
	S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error
	S3TemporaryUpload(ctx context.Context, data io.ReadCloser, size uint64, protocol StorageProtocol) (string, error)
	S3GetTemporaryUpload(ctx context.Context, protocol StorageProtocol, uploadId string) (io.ReadCloser, error)
	S3DeleteTemporaryUpload(ctx context.Context, protocol StorageProtocol, uploadId string) error
	UploadStatus(ctx context.Context, protocol StorageProtocol, objectName string) (StorageUploadStatus, *time.Time, error)

	Service
}

func NewStorageHashFromMultihashBytes(hash []byte, cidType uint64, proof []byte) StorageHash {
	multihash, err := mh.Cast(hash)

	if err != nil {
		return nil
	}

	decode, _ := mh.Decode(multihash)
	if decode == nil {
		return nil
	}

	return &StorageHashDefault{
		hash:    decode.Digest,
		typ:     decode.Code,
		proof:   proof,
		mh:      multihash,
		cidType: cidType,
	}
}

type StorageHashDefault struct {
	hash    []byte
	typ     uint64
	cidType uint64
	proof   []byte
	mh      mh.Multihash
}

func (s StorageHashDefault) Proof() []byte {
	return s.proof
}
func (s StorageHashDefault) ProofExists() bool {
	return len(s.proof) > 0
}

func (s StorageHashDefault) Multihash() mh.Multihash {
	if s.mh == nil {
		_mh, _ := mh.Encode(s.hash, s.typ)
		s.mh = _mh
	}

	return s.mh
}

func (s StorageHashDefault) CIDType() uint64 {
	return s.cidType
}

func (s StorageHashDefault) Type() uint64 {
	return s.typ
}

func (s StorageHashDefault) String() string {
	return s.Multihash().String()
}

func NewStorageHash(hash []byte, typ uint64, cidType uint64, proof []byte) StorageHash {
	return &StorageHashDefault{
		hash:    hash,
		typ:     typ,
		cidType: cidType,
		proof:   proof,
	}
}

func NewStorageHashFromMultihash(hash mh.Multihash, cidType uint64, proof []byte) StorageHash {
	decode, _ := mh.Decode(hash)
	if decode == nil {
		return nil
	}

	return &StorageHashDefault{
		hash:    decode.Digest,
		typ:     decode.Code,
		proof:   proof,
		mh:      hash,
		cidType: cidType,
	}
}

func ParseStorageHash(s string) (StorageHash, error) {
	var hash mh.Multihash
	var err error

	// Try Base58 format first (most common for content-addressed systems)
	hash, err = mh.FromB58String(s)
	if err == nil {
		// Get information from the decoded multihash
		decoded, err := mh.Decode(hash)
		if err != nil {
			return nil, fmt.Errorf("invalid multihash structure: %w", err)
		}

		cidType := inferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))
		return NewStorageHashFromMultihash(hash, cidType, nil), nil
	}

	// Try hex format
	hash, err = mh.FromHexString(s)
	if err == nil {
		decoded, err := mh.Decode(hash)
		if err != nil {
			return nil, fmt.Errorf("invalid multihash structure: %w", err)
		}

		cidType := inferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))
		return NewStorageHashFromMultihash(hash, cidType, nil), nil
	}

	// Try base64 format as a last resort
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		// Try to interpret the decoded bytes as a multihash
		hash, err = mh.Cast(decodedBytes)
		if err == nil {
			decoded, err := mh.Decode(hash)
			if err != nil {
				return nil, fmt.Errorf("invalid multihash structure: %w", err)
			}

			cidType := inferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))
			return NewStorageHashFromMultihash(hash, cidType, nil), nil
		}
	}

	// If we get here, we couldn't parse the hash in any supported format
	return nil, ErrInvalidHashFormat
}

// inferCIDTypeFromHashCode maps hash algorithm codes to appropriate CID types
// Considers both hash type and length in determining the appropriate CID type
func inferCIDTypeFromHashCode(code uint64, digestLength int) uint64 {
	// Special case for SHA2-256 with 32-byte digest (IPFS compatibility)
	if code == mh.SHA2_256 && digestLength == 32 {
		// Check if this is likely an IPFS CIDv0 hash
		return 0x00 // CIDv0 for legacy IPFS content
	}

	// For identity hashes (direct content)
	if code == mh.IDENTITY {
		return 0x55 // Raw CID for identity hashes
	}

	// For raw binary data
	if code == mh.SHA2_256 && digestLength <= 16 {
		return 0x55 // Raw CID for small binary blobs
	}

	// Default to CIDv1 for most other hash types
	return 0x01
}
