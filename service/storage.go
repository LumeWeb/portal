package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	storageMetrics "go.lumeweb.com/portal/service/internal/storage"
	"go.opentelemetry.io/otel/attribute"
	"go.sia.tech/renterd/v2/api"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.StorageService = (*StorageServiceDefault)(nil)
var _ core.StorageUploadRequest = (*StorageUploadRequestDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.STORAGE_SERVICE,
		Factory: NewStorageService,
		Depends: []string{core.RENTER_SERVICE, core.UPLOAD_SERVICE},
		Metrics: storageMetrics.GetCollectors(),
	})
}

type StorageUploadRequestDefault struct {
	protocol  core.StorageProtocol
	data      io.ReadSeeker
	size      uint64
	muParams  *core.MultipartUploadParams
	hash      core.StorageHash
	hashTypes []uint64
	hashes    []core.StorageHash
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

func (s *StorageUploadRequestDefault) SetHashTypes(types []uint64) {
	s.hashTypes = types
}

func (s StorageUploadRequestDefault) HashTypes() []uint64 {
	return s.hashTypes
}

func (s *StorageUploadRequestDefault) SetHashes(hashes []core.StorageHash) {
	s.hashes = hashes
}

func (s StorageUploadRequestDefault) Hashes() []core.StorageHash {
	return s.hashes
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
	*core.BaseComponent
	renter   core.RenterService
	metadata core.UploadService
}

func NewStorageService() (core.Service, []core.ContextBuilderOption, error) {
	storage := &StorageServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			storage.renter = core.GetService[core.RenterService](ctx, core.RENTER_SERVICE)
			storage.metadata = core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
			return nil
		}),
	)

	return storage, opts, nil
}

func (s StorageServiceDefault) ID() string {
	return core.STORAGE_SERVICE
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

func (s StorageServiceDefault) UploadObject(ctx context.Context, request core.StorageUploadRequest) (*models.Upload, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.UploadObject")
	defer span.End()

	startTime := time.Now()
	storageMetrics.ActiveUploads.Inc()
	defer storageMetrics.ActiveUploads.Dec()

	rp := newReaderPool(s.Logger())
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
			storageMetrics.UploadErrors.Inc()
			return nil, err
		}
		hash, err = s.getObjectProof(request.Protocol(), reader, request.Size())
		if err != nil {
			storageMetrics.UploadErrors.Inc()
			return nil, err
		}
	}

	span.SetAttributes(attribute.String("hash", hash.String()))

	meta, err := s.metadata.GetUpload(ctx, hash)
	if err == nil {
		storageMetrics.UploadDuration.Observe(time.Since(startTime).Seconds())
		storageMetrics.StorageCacheHits.Inc()
		return meta, nil
	}

	reader, err := getReader()
	if err != nil {
		storageMetrics.UploadErrors.Inc()
		return nil, err
	}

	mimeType, err := s.detectMimeType(reader)
	if err != nil {
		storageMetrics.UploadErrors.Inc()
		return nil, err
	}

	protocolName := request.Protocol().Name()
	if err := s.renter.CreateBucketIfNotExists(protocolName); err != nil {
		storageMetrics.UploadErrors.Inc()
		return nil, err
	}

	filename := request.Protocol().EncodeFileName(hash)

	if hash.ProofExists() {
		if err := s.UploadObjectProof(ctx, request.Protocol(), nil, hash, request.Size()); err != nil {
			storageMetrics.UploadErrors.Inc()
			return nil, err
		}
	}

	uploadMeta := &models.Upload{
		Protocol: protocolName,
		Hash:     hash.Multihash(),
		CIDType:  hash.CIDType(),
		MimeType: mimeType.String(),
		Size:     request.Size(),
	}

	if params := request.MuParams(); params != nil {
		params.FileName = filename
		params.Bucket = protocolName
		params.Size = request.Size()
		if err := s.renter.UploadObjectMultipart(ctx, params); err != nil {
			storageMetrics.UploadErrors.Inc()
			return uploadMeta, err
		}
		storageMetrics.UploadDuration.Observe(time.Since(startTime).Seconds())
		storageMetrics.UploadBytes.Add(float64(request.Size()))
		return uploadMeta, nil
	}

	reader, err = getReader()
	if err != nil {
		storageMetrics.UploadErrors.Inc()
		return nil, err
	}

	if err := s.renter.UploadObject(ctx, reader, protocolName, filename); err != nil {
		storageMetrics.UploadErrors.Inc()
		return uploadMeta, err
	}

	storageMetrics.UploadDuration.Observe(time.Since(startTime).Seconds())
	storageMetrics.UploadBytes.Add(float64(request.Size()))
	return uploadMeta, nil
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
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.UploadObjectProof")
	defer span.End()

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

func (s StorageServiceDefault) applyStorageOptions(opts []core.StorageOptionFunc) *core.StorageOption {
	options := &core.StorageOption{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

func (s StorageServiceDefault) DownloadObject(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash, start int64) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.DownloadObject",
		core.WithAttributes(attribute.String("hash", objectHash.String())))
	defer span.End()

	return s.DownloadObjectWithOptions(ctx, protocol, objectHash, core.StorageDownloadWithStart(start))
}

func (s StorageServiceDefault) DownloadObjectWithOptions(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash, opts ...core.StorageOptionFunc) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.DownloadObjectWithOptions",
		core.WithAttributes(attribute.String("hash", objectHash.String())))
	defer span.End()

	startTime := time.Now()

	var partialRange *api.DownloadRange = nil
	options := s.applyStorageOptions(opts)

	if !options.SkipMetadataCheck {
		upload, err := s.metadata.GetUpload(ctx, objectHash)
		if err != nil {
			storageMetrics.DownloadErrors.Inc()
			return nil, err
		}

		if options.Start > 0 {
			partialRange = &api.DownloadRange{
				Offset: options.Start,
				Length: int64(upload.Size) - options.Start + 1,
			}
		}
	}

	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash), api.DownloadObjectOptions{Range: partialRange})
	if err != nil {
		storageMetrics.DownloadErrors.Inc()
		return nil, err
	}

	storageMetrics.DownloadDuration.Observe(time.Since(startTime).Seconds())

	return object.Content, nil
}

func (s StorageServiceDefault) DownloadObjectProof(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.DownloadObjectProof",
		core.WithAttributes(attribute.String("hash", objectHash.String())))
	defer span.End()

	object, err := s.renter.GetObject(ctx, protocol.Name(), s.getProofPath(protocol, objectHash), api.DownloadObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DeleteObject(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.DeleteObject",
		core.WithAttributes(attribute.String("hash", objectHash.String())))
	defer span.End()

	return core.MetricTrack(storageMetrics.DeleteDuration, storageMetrics.DeleteErrors, func() error {
		return s.renter.DeleteObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash))
	})
}

func (s StorageServiceDefault) DeleteObjectProof(ctx context.Context, protocol core.StorageProtocol, objectHash core.StorageHash) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.DeleteObjectProof",
		core.WithAttributes(attribute.String("hash", objectHash.String())))
	defer span.End()

	return core.MetricTrack(storageMetrics.DeleteDuration, storageMetrics.DeleteErrors, func() error {
		return s.renter.DeleteObject(ctx, protocol.Name(), s.getProofPath(protocol, objectHash))
	})
}

// S3Upload uploads an object to S3 storage.
// bucket: The S3 bucket name
// key: The object key/path
// data: The data to upload
// Returns error if upload fails
func (s StorageServiceDefault) S3Upload(ctx context.Context, bucket string, key string, data io.Reader) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3Upload")
	defer span.End()

	return s.s3PutObject(ctx, bucket, key, data, 0)
}

// S3Delete deletes an object from S3 storage.
// bucket: The S3 bucket name
// key: The object key/path to delete
// Returns error if deletion fails
func (s StorageServiceDefault) S3Delete(ctx context.Context, bucket string, key string) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3Delete")
	defer span.End()

	return s.s3DeleteObject(ctx, bucket, key)
}

// S3Download downloads an object from S3 storage.
// bucket: The S3 bucket name
// key: The object key/path to download
// Returns io.ReadSeekCloser for the object data (caller must close it) and error if download fails
func (s StorageServiceDefault) S3Download(ctx context.Context, bucket string, key string) (io.ReadSeekCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3Download")
	defer span.End()

	return s.s3GetObject(ctx, bucket, key)
}

// s3PutObject is an internal helper for putting objects to S3 storage.
// bucket: The S3 bucket name
// key: The object key/path
// data: The data to upload
// size: The size of the data in bytes
// Returns error if put operation fails
func (s StorageServiceDefault) s3PutObject(ctx context.Context, bucket string, key string, data io.Reader, size int64) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.s3PutObject")
	defer span.End()

	var uploadSize uint64
	if size > 0 {
		uploadSize = uint64(size)
	}

	var uploadErr error
	core.MetricTrackWithBytes(storageMetrics.S3UploadDuration, storageMetrics.S3UploadBytes, storageMetrics.S3UploadErrors, func() (bool, uint64, error) {
		client, err := s.S3Client(ctx)
		if err != nil {
			uploadErr = err
			return false, uploadSize, err
		}

		input := &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   data,
		}

		// Set ContentLength if size is known or can be determined
		if size > 0 {
			input.ContentLength = aws.Int64(size)
		} else {
			switch r := data.(type) {
			case *bytes.Reader:
				input.ContentLength = aws.Int64(r.Size())
				uploadSize = uint64(r.Size())
			case *strings.Reader:
				input.ContentLength = aws.Int64(r.Size())
				uploadSize = uint64(r.Size())
			}
		}

		// Try to detect content type if available
		if seeker, ok := data.(io.ReadSeeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err == nil {
				if mime, err := s.detectMimeType(seeker); err == nil {
					input.ContentType = aws.String(mime.String())
				}
				// Reset reader position
				if _, err := seeker.Seek(0, io.SeekStart); err != nil {
					uploadErr = fmt.Errorf("failed to reset reader after MIME detection: %w", err)
					return false, uploadSize, uploadErr
				}
			}
		}

		_, err = client.PutObject(ctx, input)
		uploadErr = err
		return err == nil, uploadSize, err
	})
	return uploadErr
}

// s3GetObject is an internal helper for getting objects from S3 storage.
// bucket: The S3 bucket name
// key: The object key/path
// Returns io.ReadSeekCloser for the object data and error if get operation fails
func (s StorageServiceDefault) s3GetObject(ctx context.Context, bucket string, key string) (io.ReadSeekCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.s3GetObject")
	defer span.End()

	return core.MetricTrackResult(storageMetrics.S3DownloadDuration, storageMetrics.S3DownloadErrors, func() (io.ReadSeekCloser, error) {
		client, err := s.S3Client(ctx)
		if err != nil {
			return nil, err
		}

		// Create S3Reader with fixed chunk size policy
		chunkPolicy := &storageMetrics.FixedChunkSizePolicy{Size: 1024 * 1024} // 1MB chunks
		return storageMetrics.NewS3Reader(ctx, s.Logger(), client, bucket, key, chunkPolicy)
	})
}

// s3DeleteObject is an internal helper for deleting objects from S3 storage.
// bucket: The S3 bucket name
// key: The object key/path to delete
// Returns error if delete operation fails
func (s StorageServiceDefault) s3DeleteObject(ctx context.Context, bucket string, key string) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.s3DeleteObject")
	defer span.End()

	return core.MetricTrack(storageMetrics.S3DeleteDuration, storageMetrics.S3DeleteErrors, func() error {
		client, err := s.S3Client(ctx)
		if err != nil {
			return err
		}

		_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		return err
	})
}

// S3Client creates and returns a new S3 client instance.
// Returns configured *s3.Client and error if client creation fails
func (s StorageServiceDefault) S3Client(ctx context.Context) (*s3.Client, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3Client")
	defer span.End()

	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(s.Config().Config().Core.Storage.S3.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.Config().Config().Core.Storage.S3.AccessKey,
			s.Config().Config().Core.Storage.S3.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(ensureHttpPrefix(s.Config().Config().Core.Storage.S3.Endpoint))
		o.UsePathStyle = true
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	}), nil
}

// S3MultipartUpload performs a multipart upload to S3 storage.
// data: The data to upload
// bucket: The S3 bucket name
// key: The object key/path
// size: The total size of the data in bytes
// Returns error if upload fails
func (s StorageServiceDefault) S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3MultipartUpload")
	defer span.End()

	var uploadErr error
	core.MetricTrackGaugeWithBytes(storageMetrics.ActiveUploads, storageMetrics.S3UploadDuration, storageMetrics.S3UploadBytes, storageMetrics.S3UploadErrors, func() (bool, uint64, error) {
		client, err := s.S3Client(ctx)
		if err != nil {
			uploadErr = err
			return false, size, err
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

		err = s.renter.CreateBucketIfNotExists(bucket)
		if err != nil {
			uploadErr = err
			return false, size, err
		}

		var totalUploadDuration time.Duration
		var currentAverageDuration time.Duration

		if err = db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
			return tx.Model(&s3Upload).First(&s3Upload)
		}); err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				uploadErr = err
				return false, size, err
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
				uploadErr = err
				return false, size, err
			}

			uploadId = *mu.UploadId

			s3Upload.UploadID = uploadId
			if err = db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Create(&s3Upload)
			}); err != nil {
				uploadErr = err
				return false, size, err
			}
		}

		for partNum := 1; partNum <= totalParts; partNum++ {
			partStartTime := time.Now()
			partData := make([]byte, partSize)
			readSize, err := data.Read(partData)
			if err != nil && err != io.EOF {
				uploadErr = err
				return false, size, err
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
				storageMetrics.MultipartUploadErrors.Inc()
				// Abort the multipart upload in case of error
				_, abortErr := client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
					Bucket:   aws.String(bucket),
					Key:      aws.String(key),
					UploadId: aws.String(uploadId),
				})
				if abortErr != nil {
					s.Logger().Error("error aborting multipart upload", zap.Error(abortErr))
				}
				uploadErr = err
				return false, size, err
			}

			completedParts = append(completedParts, types.CompletedPart{
				ETag:       uploadPartOutput.ETag,
				PartNumber: aws.Int32(int32(partNum)),
			})

			storageMetrics.MultipartUploadParts.Inc()

			partDuration := time.Since(partStartTime)
			totalUploadDuration += partDuration

			currentAverageDuration = totalUploadDuration / time.Duration(partNum)

			eta := time.Duration(int(currentAverageDuration) * (totalParts - partNum))

			s.Logger().Debug("Completed part",
				zap.Int("partNum", partNum),
				zap.Int("totalParts", totalParts),
				zap.Uint64("partSize", partSize),
				zap.Int("readSize", readSize),
				zap.Uint64("size", size),
				zap.String("key", key),
				zap.String("bucket", bucket),
				zap.Duration("duration", partDuration),
				zap.Duration("currentAverageDuration", currentAverageDuration),
				zap.Duration("eta", eta),
			)
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
			storageMetrics.MultipartUploadErrors.Inc()
			uploadErr = err
			return false, size, err
		}

		if err = db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
			return tx.Delete(&s3Upload)
		}); err != nil {
			s.Logger().Error("failed to delete S3Upload record after successful upload", zap.Error(err), zap.String("bucket", bucket), zap.String("key", key))
			// Don't return error since upload succeeded
		}

		s.Logger().Debug("S3 multipart upload complete", zap.String("key", key), zap.String("bucket", bucket))

		return true, size, nil
	})

	return uploadErr
}

func (s StorageServiceDefault) UploadStatus(ctx context.Context, protocol core.StorageProtocol, objectName string) (core.StorageUploadStatus, *time.Time, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.UploadStatus")
	defer span.End()

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

// S3TemporaryUpload uploads data to temporary S3 storage.
// data: The data to upload
// size: The size of the data in bytes
// protocol: The storage protocol being used
// opts: Optional configuration functions (e.g., WithS3TempUploadID)
// Returns upload ID and error if upload fails
// validateUploadID checks if uploadId contains path traversal sequences
func validateUploadID(uploadId string) error {
	if strings.Contains(uploadId, "..") || strings.Contains(uploadId, "\\") {
		return fmt.Errorf("invalid upload ID: must not contain path traversal sequences or backslashes")
	}
	return nil
}

func (s StorageServiceDefault) S3TemporaryUpload(ctx context.Context, data io.ReadCloser, size uint64, protocol core.StorageProtocol, opts ...func(*core.S3TempUploadOptions)) (string, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3TemporaryUpload")
	defer span.End()

	options := &core.S3TempUploadOptions{}
	for _, opt := range opts {
		opt(options)
	}

	uploadId := options.UploadID
	// Validate custom uploadId to prevent path traversal
	if uploadId != "" {
		if err := validateUploadID(uploadId); err != nil {
			return "", err
		}
	}
	if uploadId == "" {
		uploadId = uuid.NewString()
	}

	key := s.GetTemporaryUploadPath(protocol, uploadId)

	defer func(data io.ReadCloser) {
		err := data.Close()
		if err != nil {
			s.Logger().Error("error closing data", zap.Error(err))
		}
	}(data)

	err := s.s3PutObject(ctx, s.Config().Config().Core.Storage.S3.BufferBucket, key, data, int64(size))
	if err != nil {
		return "", err
	}

	return uploadId, nil
}

// S3GetTemporaryUpload retrieves a temporary upload from S3 storage.
// protocol: The storage protocol being used
// uploadId: The ID of the temporary upload
// Returns io.ReadSeekCloser for the upload data and error if retrieval fails
func (s StorageServiceDefault) S3GetTemporaryUpload(ctx context.Context, protocol core.StorageProtocol, uploadId string) (io.ReadSeekCloser, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3GetTemporaryUpload")
	defer span.End()

	// Validate uploadId to prevent path traversal
	if err := validateUploadID(uploadId); err != nil {
		return nil, err
	}
	return s.s3GetObject(ctx, s.Config().Config().Core.Storage.S3.BufferBucket, s.GetTemporaryUploadPath(protocol, uploadId))
}

// S3DeleteTemporaryUpload deletes a temporary upload from S3 storage.
// protocol: The storage protocol being used
// uploadId: The ID of the temporary upload to delete
// Returns error if deletion fails
func (s StorageServiceDefault) S3Exists(ctx context.Context, bucket string, key string) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3Exists")
	defer span.End()

	client, err := s.S3Client(ctx)
	if err != nil {
		return false, err
	}

	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		var apiErr smithy.APIError
		if errors.As(err, &notFound) ||
			(errors.As(err, &apiErr) &&
				(apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey")) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s StorageServiceDefault) S3DeleteTemporaryUpload(ctx context.Context, protocol core.StorageProtocol, uploadId string) error {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3DeleteTemporaryUpload")
	defer span.End()

	// Validate uploadId to prevent path traversal
	if err := validateUploadID(uploadId); err != nil {
		return err
	}
	key := s.GetTemporaryUploadPath(protocol, uploadId)

	err := s.s3DeleteObject(ctx, s.Config().Config().Core.Storage.S3.BufferBucket, key)
	if err != nil {
		return err
	}

	return nil
}

// S3TemporaryUploadExists checks if a temporary upload exists in S3 storage.
// protocol: The storage protocol being used
// uploadId: The ID of the temporary upload to check
// Returns true if the upload exists, false otherwise, and error if check fails
func (s StorageServiceDefault) S3TemporaryUploadExists(ctx context.Context, protocol core.StorageProtocol, uploadId string) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "StorageServiceDefault.S3TemporaryUploadExists")
	defer span.End()

	// Validate uploadId to prevent path traversal
	if err := validateUploadID(uploadId); err != nil {
		return false, err
	}
	key := s.GetTemporaryUploadPath(protocol, uploadId)

	return s.S3Exists(ctx, s.Config().Config().Core.Storage.S3.BufferBucket, key)
}

func (s StorageServiceDefault) getProofPath(protocol core.StorageProtocol, objectHash core.StorageHash) string {
	return fmt.Sprintf("%s%s", protocol.EncodeFileName(objectHash), core.PROOF_EXTENSION)
}

func (s StorageServiceDefault) GetTemporaryUploadPath(protocol core.StorageProtocol, uploadId string) string {
	return fmt.Sprintf("%s/%s", s.GetTemporaryUploadDir(protocol), uploadId)
}
func (s StorageServiceDefault) GetTemporaryUploadDir(protocol core.StorageProtocol) string {
	return fmt.Sprintf("%s/%s", core.TEMPORARY_UPLOADS_PATH, protocol.Name())
}

func ensureHttpPrefix(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "http://" + url
	}
	return url
}
