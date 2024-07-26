package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gabriel-vasile/mimetype"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/renterd/api"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"math"
	"sort"
	"sync"
	"time"
)

var _ core.StorageService = (*StorageServiceDefault)(nil)
var _ core.StorageHash = (*StorageHashDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.STORAGE_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewStorageService()
		},
		Depends: []string{core.RENTER_SERVICE, core.METADATA_SERVICE},
	})
}

type StorageHashDefault struct {
	hash  []byte
	typ   uint
	proof []byte
	mh    mh.Multihash
}

func (s StorageHashDefault) Proof() []byte {
	return s.proof
}
func (s StorageHashDefault) ProofExists() bool {
	return len(s.proof) > 0
}

func (s StorageHashDefault) Multihash() mh.Multihash {
	if s.mh == nil {
		_mh, _ := mh.Encode(s.hash, uint64(s.typ))
		s.mh = _mh
	}

	return s.mh
}

func NewStorageHash(hash []byte, typ uint, proof []byte) core.StorageHash {
	return &StorageHashDefault{
		hash:  hash,
		typ:   typ,
		proof: proof,
	}
}

type StorageUploadRequestDefault struct {
	protocol core.StorageProtocol
	data     io.ReadSeeker
	size     uint64
	muParams *core.MultipartUploadParams
	hash     core.StorageHash
}

func (s *StorageUploadRequestDefault) SetProtocol(protocol core.StorageProtocol) {
	s.protocol = protocol
}

func (s *StorageUploadRequestDefault) SetData(data io.ReadSeeker) {
	s.data = data
}

func (s *StorageUploadRequestDefault) SetSize(size uint64) {
	s.size = size
}

func (s *StorageUploadRequestDefault) SetMuParams(muParams *core.MultipartUploadParams) {
	s.muParams = muParams
}

func (s *StorageUploadRequestDefault) SetHash(hash core.StorageHash) {
	s.hash = hash
}

func (s StorageUploadRequestDefault) Protocol() core.StorageProtocol {
	return s.protocol
}

func (s StorageUploadRequestDefault) Data() io.ReadSeeker {
	return s.data
}

func (s StorageUploadRequestDefault) Size() uint64 {
	return s.size
}

func (s StorageUploadRequestDefault) MuParams() *core.MultipartUploadParams {
	return s.muParams
}

func (s StorageUploadRequestDefault) Hash() core.StorageHash {
	return s.hash
}

// NewStorageUploadRequest creates a new StorageUploadRequest with the given options
func NewStorageUploadRequest(options ...core.StorageUploadOption) core.StorageUploadRequest {
	request := &StorageUploadRequestDefault{}
	for _, option := range options {
		option(request)
	}
	return request
}

type StorageServiceDefault struct {
	ctx      core.Context
	config   config.Manager
	db       *gorm.DB
	renter   core.RenterService
	metadata core.MetadataService
	logger   *core.Logger
}

func NewStorageService() (*StorageServiceDefault, []core.ContextBuilderOption, error) {
	storage := &StorageServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			storage.ctx = ctx
			storage.config = ctx.Config()
			storage.db = ctx.DB()
			storage.renter = core.GetService[core.RenterService](ctx, core.RENTER_SERVICE)
			storage.metadata = core.GetService[core.MetadataService](ctx, core.METADATA_SERVICE)
			storage.logger = ctx.Logger()
			return nil
		}),
	)

	return storage, opts, nil
}

// readerPool manages a pool of readers for large, potentially non-seekable data streams
type readerPool struct {
	readers []io.ReadCloser
	mu      sync.Mutex
	logger  *core.Logger
}

// newReaderPool creates a new readerPool
func newReaderPool(logger *core.Logger) *readerPool {
	return &readerPool{
		readers: make([]io.ReadCloser, 0),
		logger:  logger,
	}
}

// GetReader returns a reader, either by creating a new one or reusing an existing one
func (rp *readerPool) GetReader(params *core.MultipartUploadParams, data io.ReadSeeker) (io.Reader, error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if params != nil {
		muReader, err := params.ReaderFactory(0, uint(params.Size))
		if err != nil {
			return nil, err
		}
		for _, r := range rp.readers {
			if r == muReader {
				return muReader, nil
			}
		}
		rp.readers = append(rp.readers, muReader)
		return muReader, nil
	}

	if _, err := data.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return data, nil
}

// Close closes all readers in the pool
func (rp *readerPool) Close() {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	for _, reader := range rp.readers {
		if err := reader.Close(); err != nil {
			rp.logger.Error("error closing reader", zap.Error(err))
		}
	}
	rp.readers = rp.readers[:0] // Clear the slice
}

func (s StorageServiceDefault) UploadObject(ctx context.Context, request core.StorageUploadRequest) (*core.UploadMetadata, error) {
	rp := newReaderPool(s.logger)
	defer rp.Close()

	getReader := func() (io.Reader, error) {
		return rp.GetReader(request.MuParams(), request.Data())
	}

	var hash core.StorageHash
	var err error

	if request.Hash() != nil {
		hash = request.Hash()
	} else {
		reader, err := getReader()
		if err != nil {
			return nil, err
		}
		hash, err = s.getObjectProof(request.Protocol(), reader, request.Size())
		if err != nil {
			return nil, err
		}
	}

	meta, err := s.metadata.GetUpload(ctx, hash)
	if err == nil {
		return &meta, nil
	}

	reader, err := getReader()
	if err != nil {
		return nil, err
	}

	mimeType, err := s.detectMimeType(reader)
	if err != nil {
		return nil, err
	}

	protocolName := request.Protocol().Name()
	if err := s.renter.CreateBucketIfNotExists(protocolName); err != nil {
		return nil, err
	}

	filename := request.Protocol().EncodeFileName(hash)

	if hash.ProofExists() {
		if err := s.UploadObjectProof(ctx, request.Protocol(), nil, hash, request.Size()); err != nil {
			return nil, err
		}
	}

	decoded, err := mh.Decode(hash.Multihash())
	if err != nil {
		return nil, err
	}

	uploadMeta := &core.UploadMetadata{
		Protocol: protocolName,
		Hash:     decoded.Digest,
		HashType: decoded.Code,
		MimeType: mimeType.String(),
		Size:     request.Size(),
	}

	if params := request.MuParams(); params != nil {
		params.FileName = filename
		params.Bucket = protocolName
		params.Size = request.Size()
		return uploadMeta, s.renter.UploadObjectMultipart(ctx, params)
	}

	reader, err = getReader()
	if err != nil {
		return nil, err
	}

	return uploadMeta, s.renter.UploadObject(ctx, reader, protocolName, filename)
}

func (s StorageServiceDefault) detectMimeType(reader io.Reader) (*mimetype.MIME, error) {
	mimeType, err := mimetype.DetectReader(reader)
	if err != nil {
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		// If we hit EOF, we'll read all available data and detect from that
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		mimeType = mimetype.Detect(data)
	}
	return mimeType, nil
}

func (s StorageServiceDefault) UploadObjectProof(ctx context.Context, protocol core.StorageProtocol, data io.ReadSeeker, proof core.StorageHash, size uint64) error {
	if proof == nil {
		hashResult, err := s.getObjectProof(protocol, data, size)
		if err != nil {
			return err
		}

		proof = hashResult
	}

	if !proof.ProofExists() {
		return core.ErrProofNotSupported
	}

	protocolName := protocol.Name()

	err := s.renter.CreateBucketIfNotExists(protocolName)

	if err != nil {
		return err
	}

	return s.renter.UploadObject(ctx, bytes.NewReader(proof.Proof()), protocolName, s.getProofPath(protocol, proof))
}

func (s StorageServiceDefault) getObjectProof(protocol core.StorageProtocol, data io.Reader, size uint64) (core.StorageHash, error) {
	hashResult, err := protocol.Hash(data, size)
	if err != nil {
		return nil, err
	}

	if !hashResult.ProofExists() {
		return nil, core.ErrProofNotSupported
	}

	return hashResult, nil
}

func (s StorageServiceDefault) DownloadObject(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash, start int64) (io.ReadCloser, error) {
	var partialRange *api.DownloadRange = nil

	upload, err := s.metadata.GetUpload(ctx, objectHash)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		partialRange = &api.DownloadRange{
			Offset: start,
			Length: int64(upload.Size) - start + 1,
		}
	}

	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash), api.DownloadObjectOptions{Range: partialRange})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DownloadObjectProof(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) (io.ReadCloser, error) {
	object, err := s.renter.GetObject(ctx, protocol.Name(), s.getProofPath(protocol, objectHash), api.DownloadObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DeleteObject(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) error {
	err := s.renter.DeleteObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash))
	if err != nil {
		return err
	}

	return nil
}

func (s StorageServiceDefault) DeleteObjectProof(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) error {
	err := s.renter.DeleteObject(ctx, protocol.Name(), s.getProofPath(protocol, objectHash))
	if err != nil {
		return err
	}

	return nil
}

func (s StorageServiceDefault) S3Client(ctx context.Context) (*s3.Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           s.config.Config().Core.Storage.S3.Endpoint,
				SigningRegion: s.config.Config().Core.Storage.S3.Region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion("us-east-1"),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.config.Config().Core.Storage.S3.AccessKey,
			s.config.Config().Core.Storage.S3.SecretKey,
			"",
		)),
		awsConfig.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}

func (s StorageServiceDefault) S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error {
	client, err := s.S3Client(ctx)
	if err != nil {
		return err
	}

	var uploadId string
	var lastPartNumber int32

	partSize := core.S3_MULTIPART_MIN_PART_SIZE
	totalParts := int(math.Ceil(float64(size) / float64(partSize)))
	if totalParts > core.S3_MULTIPART_MAX_PARTS {
		partSize = size / core.S3_MULTIPART_MAX_PARTS
		totalParts = core.S3_MULTIPART_MAX_PARTS
	}

	var completedParts []types.CompletedPart

	var s3Upload models.S3Upload

	s3Upload.Bucket = bucket
	s3Upload.Key = key

	startTime := time.Now()
	var totalUploadDuration time.Duration
	var currentAverageDuration time.Duration

	if err = db.RetryOnLock(s.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&s3Upload).First(&s3Upload)
	}); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else {
		uploadId = s3Upload.UploadID
	}

	if len(uploadId) > 0 {
		parts, err := client.ListParts(ctx, &s3.ListPartsInput{
			Bucket:   aws.String(bucket),
			Key:      aws.String(key),
			UploadId: aws.String(uploadId),
		})

		if err != nil {
			uploadId = ""
		} else {
			for _, part := range parts.Parts {
				if uint64(*part.Size) == partSize {
					if *part.PartNumber > lastPartNumber {
						lastPartNumber = *part.PartNumber
						completedParts = append(completedParts, types.CompletedPart{
							ETag:       part.ETag,
							PartNumber: part.PartNumber,
						})
					}
				}
			}
		}
	}

	if uploadId == "" {
		mu, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return err
		}

		uploadId = *mu.UploadId

		s3Upload.UploadID = uploadId
		if err = db.RetryOnLock(s.db, func(db *gorm.DB) *gorm.DB {
			return db.Create(&s3Upload)
		}); err != nil {
			return err
		}
	}

	for partNum := 1; partNum <= totalParts; partNum++ {
		partStartTime := time.Now()
		partData := make([]byte, partSize)
		readSize, err := data.Read(partData)
		if err != nil && err != io.EOF {
			return err
		}

		if partNum <= int(lastPartNumber) {
			continue
		}
		uploadPartOutput, err := client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			PartNumber: aws.Int32(int32(partNum)),
			UploadId:   aws.String(uploadId),
			Body:       bytes.NewReader(partData[:readSize]),
		})
		if err != nil {
			// Abort the multipart upload in case of error
			_, abortErr := client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(bucket),
				Key:      aws.String(key),
				UploadId: aws.String(uploadId),
			})
			if abortErr != nil {
				s.logger.Error("error aborting multipart upload", zap.Error(abortErr))
			}
			return err
		}

		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadPartOutput.ETag,
			PartNumber: aws.Int32(int32(partNum)),
		})

		partDuration := time.Since(partStartTime)
		totalUploadDuration += partDuration

		currentAverageDuration = totalUploadDuration / time.Duration(partNum)

		eta := time.Duration(int(currentAverageDuration) * (totalParts - partNum))

		s.logger.Debug("Completed part", zap.Int("partNum", partNum), zap.Int("totalParts", totalParts), zap.Uint64("partSize", partSize), zap.Int("readSize", readSize), zap.Int("size", int(size)), zap.Int("totalParts", totalParts), zap.Int("partNum", partNum), zap.String("key", key), zap.String("bucket", bucket), zap.Duration("durationMs", partDuration),
			zap.Duration("currentAverageDurationMs", currentAverageDuration), zap.Duration("eta", eta))
	}

	// Ensure parts are ordered by part number before completing the upload
	sort.Slice(completedParts, func(i, j int) bool {
		return *completedParts[i].PartNumber < *completedParts[j].PartNumber
	})

	_, err = client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadId),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return err
	}

	if err = db.RetryOnLock(s.db, func(db *gorm.DB) *gorm.DB {
		return db.Delete(&s3Upload)
	}); err != nil {
		return err
	}

	endTime := time.Now()
	s.logger.Debug("S3 multipart upload complete", zap.String("key", key), zap.String("bucket", bucket), zap.Duration("duration", endTime.Sub(startTime)))

	return nil
}

func (s StorageServiceDefault) UploadStatus(ctx context.Context, protocol core.StorageProtocol, objectName string) (core.StorageUploadStatus, *time.Time, error) {
	exists, upload, err := s.renter.UploadExists(ctx, protocol.Name(), objectName)
	if err != nil {
		return core.StorageUploadStatusUnknown, nil, err
	}

	if exists {
		lastModified := upload.UpdatedAt
		return core.StorageUploadStatusActive, &lastModified, nil
	}

	return core.StorageUploadStatusProcessing, nil, nil

}

func (s StorageServiceDefault) getProofPath(protocol core.StorageProtocol, objectHash core.StorageHash) string {
	return fmt.Sprintf("%s%s", protocol.EncodeFileName(objectHash), core.PROOF_EXTENSION)
}
