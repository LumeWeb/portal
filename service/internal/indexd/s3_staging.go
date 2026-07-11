package indexd

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.opentelemetry.io/otel/attribute"
)

// S3StagingBackend implements StagingBackend using S3 storage.
//
// When configured with a dedicated S3StagingConfig (Core.Storage.Sia.S3Staging),
// it uses that config's endpoint, bucket, and credentials. When the staging
// config is empty, it falls back to the main S3 config (Core.Storage.S3) and
// uses BufferBucket.
//
// Key design: stagingKey is the S3 object key. Format: "staging/{uuid}".
type S3StagingBackend struct {
	s3Client *s3.Client
	bucket   string
}

// NewS3StagingBackend creates a staging backend from the sia S3 staging config.
// If the staging config is empty, it falls back to the main S3 config.
func NewS3StagingBackend(ctx context.Context, siaCfg config.SiaConfig, s3Cfg config.S3Config, endpointFn func(string) string) (*S3StagingBackend, error) {
	// Determine which config to use: dedicated staging config with
	// per-field fallback to main S3 config.
	endpoint := siaCfg.S3Staging.Endpoint
	if endpoint == "" {
		endpoint = s3Cfg.Endpoint
	}
	region := siaCfg.S3Staging.Region
	if region == "" {
		region = s3Cfg.Region
	}
	accessKey := siaCfg.S3Staging.AccessKey
	if accessKey == "" {
		accessKey = s3Cfg.AccessKey
	}
	secretKey := siaCfg.S3Staging.SecretKey
	if secretKey == "" {
		secretKey = s3Cfg.SecretKey
	}
	bucket := siaCfg.S3Staging.Bucket
	if bucket == "" {
		bucket = s3Cfg.BufferBucket
	}

	if bucket == "" {
		return nil, fmt.Errorf("staging bucket is not configured: set core.storage.sia.s3_staging.bucket or core.storage.s3.buffer_bucket")
	}

	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey,
			secretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for staging backend: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointFn(endpoint))
		o.UsePathStyle = true
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &S3StagingBackend{
		s3Client: client,
		bucket:   bucket,
	}, nil
}

// NewS3StagingBackendFromClient creates a staging backend from an existing S3
// client. Used for testing.
func NewS3StagingBackendFromClient(s3Client *s3.Client, bucket string) *S3StagingBackend {
	return &S3StagingBackend{
		s3Client: s3Client,
		bucket:   bucket,
	}
}

// Put uploads the reader data to S3 with a unique key and returns the key.
func (s *S3StagingBackend) Put(ctx context.Context, reader io.Reader) (string, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.S3StagingBackend.Put")
	defer span.End()
	stagingKey := fmt.Sprintf("staging/%s", uuid.NewString())
	span.SetAttributes(attribute.String("indexd.stagingKey", stagingKey))

	_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
		Body:   reader,
	})
	if err != nil {
		return "", fmt.Errorf("failed to put staging object: %w", err)
	}

	return stagingKey, nil
}

// Get retrieves staged object data from S3. If offset/length are specified,
// a range read is performed. offset=0, length=-1 means read all.
func (s *S3StagingBackend) Get(ctx context.Context, stagingKey string, offset, length int64) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.S3StagingBackend.Get")
	defer span.End()
	span.SetAttributes(
		attribute.String("indexd.stagingKey", stagingKey),
		attribute.Int64("indexd.offset", offset),
		attribute.Int64("indexd.length", length),
	)
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
	}

	if length >= 0 {
		lastByte := offset + length - 1
		input.Range = aws.String(fmt.Sprintf("bytes=%d-%d", offset, lastByte))
	} else if offset > 0 {
		input.Range = aws.String(fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := s.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get staging object: %w", err)
	}

	return resp.Body, nil
}

// Delete removes the staged object data from S3.
func (s *S3StagingBackend) Delete(ctx context.Context, stagingKey string) error {
	ctx, span := core.TraceMethod(ctx, "indexd.S3StagingBackend.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("indexd.stagingKey", stagingKey))
	_, err := s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete staging object: %w", err)
	}

	return nil
}

// Size returns the byte size of the staged object via HeadObject.
func (s *S3StagingBackend) Size(ctx context.Context, stagingKey string) (int64, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.S3StagingBackend.Size")
	defer span.End()
	span.SetAttributes(attribute.String("indexd.stagingKey", stagingKey))
	resp, err := s.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get staging object size: %w", err)
	}

	if resp.ContentLength == nil {
		return 0, nil
	}

	return *resp.ContentLength, nil
}
