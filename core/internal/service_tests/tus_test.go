package service_tests

import (
	"errors"
	"testing"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUSService_UploadExists(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create mock request service
		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Setup mock expectations
		requestService.EXPECT().RegisterRequestModel(core.TUSUploadOperationName("test"), &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")

		// Test upload exists
		exists, retrievedTUSRequest := tusService.UploadExists(ctx, mockProto, "test_upload_id")
		assert.True(tb, exists)
		assert.Equal(tb, tusRequest.TUSUploadID, retrievedTUSRequest.TUSUploadID)

		// Test upload does not exist
		exists, _ = tusService.UploadExists(ctx, mockProto, "nonexistent_upload_id")
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestTUSService_UploadProcessing(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
			Status:    models.RequestStatusPending,
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")

		// Test upload processing
		err = tusService.UploadProcessing(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request status is updated
		var updatedRequest models.Request
		err = ctx.DB().First(&updatedRequest, request.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusProcessing, updatedRequest.Status)

		// Test upload does not exist
		err = tusService.UploadProcessing(ctx, mockProto, "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestTUSService_UploadCompleted(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create mock request service
		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Setup mock expectations
		requestService.On("RegisterRequestModel", core.TUSUploadOperationName("test"), &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
			Status:    models.RequestStatusProcessing,
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")

		// Test upload completed
		err = tusService.UploadCompleted(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request status is updated
		var updatedRequest models.Request
		err = ctx.DB().First(&updatedRequest, request.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, updatedRequest.Status)

		// Test upload does not exist
		err = tusService.UploadCompleted(ctx, mockProto, "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestTUSService_DeleteUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create mock request service
		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Setup mock expectations
		requestService.On("RegisterRequestModel", core.TUSUploadOperationName("test"), &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")

		// Test delete upload
		err = tusService.DeleteUpload(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request is deleted
		var deletedRequest models.Request
		err = ctx.DB().First(&deletedRequest, request.ID).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

		// Test upload does not exist
		err = tusService.DeleteUpload(ctx, mockProto, "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}
