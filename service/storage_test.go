package service

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/renterd/v2/api"
	"testing"

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
		uploadRequest := NewStorageUploadRequest(
			core.StorageUploadWithProtocol(mockProtocol),
			core.StorageUploadWithData(testData),
			core.StorageUploadWithSize(uint64(testData.Len())),
		)

		// Call UploadObject
		_, err := storageService.UploadObject(context.Background(), uploadRequest)
		assert.Error(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
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
		uploadService := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)
		uploadService.EXPECT().GetUpload(mock.Anything, mockStorageHash).Return(upload, nil)

		renterService := core.GetService[*coreMocks.MockRenterService](ctx, core.RENTER_SERVICE)
		renterService.EXPECT().GetObject(
			mock.Anything,
			mockProtocol.Name(),
			mockStorageHash.String(),
			mock.AnythingOfType("api.DownloadObjectOptions"),
		).Return(&api.GetObjectResponse{}, nil)

		// Call DownloadObject
		_, err = storageService.DownloadObject(context.Background(), mockProtocol, mockStorageHash, 0)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
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
		renterService := core.GetService[*coreMocks.MockRenterService](ctx, core.RENTER_SERVICE)
		renterService.EXPECT().DeleteObject(mock.Anything, mockProtocol.Name(), mockStorageHash.String()).Return(nil)

		// Call DeleteObject
		err := storageService.DeleteObject(context.Background(), mockProtocol, mockStorageHash)
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.STORAGE_SERVICE, NewStorageService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}
