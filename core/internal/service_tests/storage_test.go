package service_tests

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageService_UploadObject(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock protocol
		mockProtocol := coreTesting.NewMockProtocol(t, "testprotocol")

		// Test data
		testData := bytes.NewReader([]byte("test data"))

		// Create upload request
		uploadRequest := service.NewStorageUploadRequest(
			core.StorageUploadWithProtocol(mockProtocol),
			core.StorageUploadWithData(testData),
			core.StorageUploadWithSize(uint64(testData.Len())),
		)

		// Call UploadObject
		_, err := storageService.UploadObject(context.Background(), uploadRequest)
		assert.Error(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, service.NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestStorageService_DownloadObject(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock protocol
		mockProtocol := coreTesting.NewMockProtocol(t, "testprotocol")

		// Mock storage hash
		mockStorageHash := coreTesting.NewMockStorageHash()

		// Create a test upload
		upload := &models.Upload{
			Protocol: "testprotocol",
			Hash:     []byte("test_hash"),
			Size:     100,
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// Setup mock services
		uploadService := coreTesting.GetMockUploadService(ctx)
		uploadService.EXPECT().GetUpload(mock.Anything, mockStorageHash).Return(upload, nil)

		renterService := coreTesting.GetMockRenterService(ctx)
		renterService.EXPECT().GetObject(
			mock.Anything,
			mockProtocol.Name(),
			mockStorageHash.String(),
			mock.AnythingOfType("core.DownloadOptions"),
		).Return(io.NopCloser(bytes.NewReader([]byte{})), nil)

		// Call DownloadObject
		_, err = storageService.DownloadObject(context.Background(), mockProtocol, mockStorageHash, 0)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, service.NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestStorageService_DeleteObject(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock protocol
		mockProtocol := coreTesting.NewMockProtocol(t, "testprotocol")

		// Mock storage hash
		mockStorageHash := coreTesting.NewMockStorageHash()

		// Setup mock RenterService
		renterService := coreTesting.GetMockRenterService(ctx)
		renterService.EXPECT().DeleteObject(mock.Anything, mockProtocol.Name(), mockStorageHash.String()).Return(nil)

		// Call DeleteObject
		err := storageService.DeleteObject(context.Background(), mockProtocol, mockStorageHash)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, service.NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

/*
func TestStorageService_S3Upload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock S3 client
		mockS3 := coreTesting.NewMockS3Client()
		mockS3.EXPECT().PutObject(mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

		// Test data
		testData := bytes.NewReader([]byte("test data"))

		// Test upload
		err := storageService.S3Upload(context.Background(), "test-bucket", "test-key", testData)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestStorageService_S3Download(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock S3 client
		mockS3 := coreTesting.NewMockS3Client()
		mockS3.EXPECT().GetObject(mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte("test data"))),
		}, nil)

		// Test download
		reader, err := storageService.S3Download(context.Background(), "test-bucket", "test-key")
		assert.NoError(tb, err)
		assert.NotNil(tb, reader)
		defer reader.Close()

		data, err := io.ReadAll(reader)
		assert.NoError(tb, err)
		assert.Equal(t, []byte("test data"), data)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestStorageService_S3Delete(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock S3 client
		mockS3 := coreTesting.NewMockS3Client()
		mockS3.EXPECT().DeleteObject(mock.Anything, mock.Anything).Return(&s3.DeleteObjectOutput{}, nil)

		// Test delete
		err := storageService.S3Delete(context.Background(), "test-bucket", "test-key")
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestStorageService_S3MultipartUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		storageService := core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE)
		require.NotNil(tb, storageService)

		// Mock S3 client
		mockS3 := coreTesting.NewMockS3Client()
		mockS3.EXPECT().CreateMultipartUpload(mock.Anything, mock.Anything).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("test-upload-id"),
		}, nil)
		mockS3.EXPECT().UploadPart(mock.Anything, mock.Anything).Return(&s3.UploadPartOutput{
			ETag: aws.String("test-etag"),
		}, nil)
		mockS3.EXPECT().CompleteMultipartUpload(mock.Anything, mock.Anything).Return(&s3.CompleteMultipartUploadOutput{}, nil)

		// Test data
		testData := io.NopCloser(bytes.NewReader([]byte("test data")))

		// Test multipart upload
		err := storageService.S3MultipartUpload(context.Background(), testData, "test-bucket", "test-key", 9)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}
*/
