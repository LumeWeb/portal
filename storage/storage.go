package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/docker/go-units"

	"github.com/aws/aws-sdk-go-v2/aws"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/fx"

	"go.sia.tech/renterd/api"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"go.uber.org/zap"

	"git.lumeweb.com/LumeWeb/portal/bao"

	"gorm.io/gorm"

	"git.lumeweb.com/LumeWeb/portal/renter"
)

const PROOF_EXTENSION = ".obao"
const S3_MULTIPART_MAX_PARTS = 9500
const S3_MULTIPART_MIN_PART_SIZE = uint64(5 * units.MiB)

var _ StorageService = (*StorageServiceDefault)(nil)

type FileNameEncoderFunc func([]byte) string

type StorageProtocol interface {
	Name() string
	EncodeFileName([]byte) string
}

var Module = fx.Module("storage",
	fx.Provide(
		fx.Annotate(
			NewStorageService,
			fx.As(new(StorageService)),
		),
	),
)

type StorageService interface {
	UploadObject(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, muParams *renter.MultiPartUploadParams, proof *bao.Result) (*metadata.UploadMetadata, error)
	UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof *bao.Result) error
	HashObject(ctx context.Context, data io.Reader) (*bao.Result, error)
	DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash []byte, start int64) (io.ReadCloser, error)
	DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
	DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
	S3Client(ctx context.Context) (*s3.Client, error)
	S3MultipartUpload(ctx context.Context, data io.ReadCloser, bucket, key string, size uint64) error
}

type StorageServiceDefault struct {
	config   *config.Manager
	db       *gorm.DB
	renter   *renter.RenterDefault
	logger   *zap.Logger
	metadata metadata.MetadataService
}

type StorageServiceParams struct {
	fx.In
	Config   *config.Manager
	Db       *gorm.DB
	Renter   *renter.RenterDefault
	Logger   *zap.Logger
	Metadata metadata.MetadataService
}

func NewStorageService(params StorageServiceParams) *StorageServiceDefault {
	return &StorageServiceDefault{
		config:   params.Config,
		db:       params.Db,
		renter:   params.Renter,
		logger:   params.Logger,
		metadata: params.Metadata,
	}
}

func (s StorageServiceDefault) UploadObject(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, muParams *renter.MultiPartUploadParams, proof *bao.Result) (*metadata.UploadMetadata, error) {
	readers := make([]io.ReadCloser, 0)
	defer func() {
		for _, reader := range readers {
			err := reader.Close()
			if err != nil {
				s.logger.Error("error closing reader", zap.Error(err))
			}
		}
	}()

	getReader := func() (io.Reader, error) {
		if muParams != nil {
			muReader, err := muParams.ReaderFactory(0, uint(muParams.Size))
			if err != nil {
				return nil, err
			}

			found := false
			for _, reader := range readers {
				if reader == muReader {
					found = true
					break
				}
			}

			if !found {
				readers = append(readers, muReader)
			}

			return muReader, nil
		}

		_, err := data.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	reader, err := getReader()
	if err != nil {
		return nil, err
	}

	if proof == nil {
		hashResult, err := s.HashObject(ctx, reader)
		if err != nil {
			return nil, err
		}

		reader, err = getReader()
		if err != nil {
			return nil, err
		}

		proof = hashResult
	}

	meta, err := s.metadata.GetUpload(ctx, proof.Hash)
	if err == nil {
		return &meta, nil
	}

	mimeBytes := make([]byte, 512)

	reader, err = getReader()
	if err != nil {
		return nil, err
	}
	_, err = io.ReadFull(reader, mimeBytes)
	if err != nil {
		return nil, err
	}

	reader, err = getReader()
	if err != nil {
		return nil, err
	}

	mimeType := http.DetectContentType(mimeBytes)

	protocolName := protocol.Name()

	err = s.renter.CreateBucketIfNotExists(protocolName)
	if err != nil {
		return nil, err
	}

	filename := protocol.EncodeFileName(proof.Hash)

	err = s.UploadObjectProof(ctx, protocol, nil, proof)

	if err != nil {
		return nil, err
	}

	uploadMeta := &metadata.UploadMetadata{
		Protocol: protocolName,
		Hash:     proof.Hash,
		MimeType: mimeType,
		Size:     uint64(proof.Length),
	}

	if muParams != nil {
		muParams.FileName = filename
		muParams.Bucket = protocolName

		err = s.renter.UploadObjectMultipart(ctx, muParams)
		if err != nil {
			return nil, err
		}

		return uploadMeta, nil
	}

	err = s.renter.UploadObject(ctx, reader, protocolName, filename)
	if err != nil {
		return nil, err
	}

	return uploadMeta, nil
}

func (s StorageServiceDefault) UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof *bao.Result) error {
	if proof == nil {
		hashResult, err := s.HashObject(ctx, data)
		if err != nil {
			return err
		}

		proof = hashResult
	}

	protocolName := protocol.Name()

	err := s.renter.CreateBucketIfNotExists(protocolName)

	if err != nil {
		return err
	}

	return s.renter.UploadObject(ctx, bytes.NewReader(proof.Proof), protocolName, s.getProofPath(protocol, proof.Hash))
}

func (s StorageServiceDefault) HashObject(ctx context.Context, data io.Reader) (*bao.Result, error) {
	result, err := bao.Hash(data)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s StorageServiceDefault) DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash []byte, start int64) (io.ReadCloser, error) {
	var partialRange api.DownloadRange

	upload, err := s.metadata.GetUpload(ctx, objectHash)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		partialRange = api.DownloadRange{
			Offset: start,
			Length: int64(upload.Size) - start + 1,
			Size:   int64(upload.Size),
		}
	}

	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash), api.DownloadObjectOptions{Range: partialRange})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) (io.ReadCloser, error) {
	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash)+".bao", api.DownloadObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash []byte) error {
	err := s.renter.DeleteObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash))
	if err != nil {
		return err
	}

	return nil
}

func (s StorageServiceDefault) DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) error {
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

	partSize := S3_MULTIPART_MIN_PART_SIZE
	totalParts := int(math.Ceil(float64(size) / float64(partSize)))
	if totalParts > S3_MULTIPART_MAX_PARTS {
		partSize = size / S3_MULTIPART_MAX_PARTS
		totalParts = S3_MULTIPART_MAX_PARTS
	}

	var completedParts []types.CompletedPart

	var s3Upload models.S3Upload

	s3Upload.Bucket = bucket
	s3Upload.Key = key

	startTime := time.Now()
	var totalUploadDuration time.Duration
	var currentAverageDuration time.Duration

	ret := s.db.Model(&s3Upload).First(&s3Upload)
	if ret.Error != nil {
		if !errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return ret.Error
		}
	} else {
		uploadId = s3Upload.UploadID
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
		ret = s.db.Create(&s3Upload)
		if ret.Error != nil {
			return ret.Error
		}
	} else {
		parts, err := client.ListParts(ctx, &s3.ListPartsInput{
			Bucket:   aws.String(bucket),
			Key:      aws.String(key),
			UploadId: aws.String(uploadId),
		})

		if err != nil {
			return err
		}

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

	if tx := s.db.Delete(&s3Upload); tx.Error != nil {
		return tx.Error
	}

	endTime := time.Now()
	s.logger.Debug("S3 multipart upload complete", zap.String("key", key), zap.String("bucket", bucket), zap.Duration("duration", endTime.Sub(startTime)))

	return nil
}

func (s StorageServiceDefault) getProofPath(protocol StorageProtocol, objectHash []byte) string {
	return fmt.Sprintf("%s%s", protocol.EncodeFileName(objectHash), PROOF_EXTENSION)
}
