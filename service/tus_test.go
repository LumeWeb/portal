package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
	"testing"

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
		requestService.On("RegisterRequestModel", models.RequestOperationTusUpload, &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: models.RequestOperationTusUpload,
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		// Test upload exists
		exists, retrievedTUSRequest := tusService.UploadExists(context.Background(), "test_upload_id")
		assert.True(tb, exists)
		assert.Equal(tb, tusRequest.TUSUploadID, retrievedTUSRequest.TUSUploadID)

		// Test upload does not exist
		exists, _ = tusService.UploadExists(context.Background(), "nonexistent_upload_id")
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
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
			Operation: models.RequestOperationTusUpload,
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

		// Test upload processing
		err = tusService.UploadProcessing(context.Background(), "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request status is updated
		var updatedRequest models.Request
		err = ctx.DB().First(&updatedRequest, request.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusProcessing, updatedRequest.Status)

		// Test upload does not exist
		err = tusService.UploadProcessing(context.Background(), "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSService_UploadCompleted(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create mock request service
		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Setup mock expectations
		requestService.On("RegisterRequestModel", models.RequestOperationTusUpload, &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: models.RequestOperationTusUpload,
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

		// Test upload completed
		err = tusService.UploadCompleted(context.Background(), "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request status is updated
		var updatedRequest models.Request
		err = ctx.DB().First(&updatedRequest, request.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, updatedRequest.Status)

		// Test upload does not exist
		err = tusService.UploadCompleted(context.Background(), "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSService_DeleteUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		// Create mock request service
		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Setup mock expectations
		requestService.On("RegisterRequestModel", models.RequestOperationTusUpload, &models.TUSRequest{}).Return()

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create a test Request
		request := &models.Request{
			Operation: models.RequestOperationTusUpload,
		}

		// Create the request in the database
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Set the request ID on the TUSRequest
		tusRequest.RequestID = request.ID

		// Create the TUSRequest in the database
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		// Test delete upload
		err = tusService.DeleteUpload(context.Background(), "test_upload_id")
		assert.NoError(tb, err)

		// Verify that the request is deleted
		var deletedRequest models.Request
		err = ctx.DB().First(&deletedRequest, request.ID).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

		// Test upload does not exist
		err = tusService.DeleteUpload(context.Background(), "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}
