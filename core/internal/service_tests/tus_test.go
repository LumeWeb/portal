package service_tests

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
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

		requestService := coreTesting.GetMockRequestService(ctx)
		require.NotNil(tb, requestService)

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
		}
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
			RequestID:   request.ID,
		}
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")
		opName := core.TUSUploadOperationName("test")

		// Setup mock expectations
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "test_upload_id" }),
			mock.MatchedBy(func(f core.RequestFilter) bool { return f.Operation != nil && *f.Operation == opName }),
		).Return(request, nil).Once()

		requestService.EXPECT().GetRequestData(
			mock.Anything,
			request,
		).Return(tusRequest, nil).Once()

		// Test upload exists
		exists, retrievedTUSRequest := tusService.UploadExists(ctx, mockProto, "test_upload_id")
		assert.True(tb, exists)
		assert.Equal(tb, tusRequest.TUSUploadID, retrievedTUSRequest.TUSUploadID)

		// Test upload does not exist
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "nonexistent_upload_id" }),
			mock.Anything,
		).Return(nil, gorm.ErrRecordNotFound).Once()

		exists, _ = tusService.UploadExists(ctx, mockProto, "nonexistent_upload_id")
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestTUSService_UploadProcessing(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		require.NotNil(tb, tusService)

		requestService := coreTesting.GetMockRequestService(ctx)
		require.NotNil(tb, requestService)

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
			Status:    models.RequestStatusPending,
		}
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
			RequestID:   request.ID,
		}
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")
		opName := core.TUSUploadOperationName("test")

		// UploadProcessing calls UploadExists internally
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "test_upload_id" }),
			mock.MatchedBy(func(f core.RequestFilter) bool { return f.Operation != nil && *f.Operation == opName }),
		).Return(request, nil).Once()

		requestService.EXPECT().GetRequestData(
			mock.Anything,
			request,
		).Return(tusRequest, nil).Once()

		// Then calls UpdateRequestStatus
		requestService.EXPECT().UpdateRequestStatus(
			mock.Anything,
			request.ID,
			models.RequestStatusProcessing,
			"Uploading...",
		).Return(nil).Once()

		// Test upload processing
		err = tusService.UploadProcessing(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Test upload does not exist
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "nonexistent_upload_id" }),
			mock.Anything,
		).Return(nil, gorm.ErrRecordNotFound).Once()

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

		requestService := coreTesting.GetMockRequestService(ctx)
		require.NotNil(tb, requestService)

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
			Status:    models.RequestStatusProcessing,
		}
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
			RequestID:   request.ID,
		}
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")
		opName := core.TUSUploadOperationName("test")

		// UploadCompleted calls UploadExists internally
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "test_upload_id" }),
			mock.MatchedBy(func(f core.RequestFilter) bool { return f.Operation != nil && *f.Operation == opName }),
		).Return(request, nil).Once()

		requestService.EXPECT().GetRequestData(
			mock.Anything,
			request,
		).Return(tusRequest, nil).Once()

		// Then calls GetRequest + UpdateRequestData
		requestService.EXPECT().GetRequest(
			mock.Anything,
			request.ID,
		).Return(request, nil).Once()

		requestService.EXPECT().UpdateRequestData(
			mock.Anything,
			request,
			mock.Anything,
		).Return(nil).Once()

		// Test upload completed
		err = tusService.UploadCompleted(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Test upload does not exist
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "nonexistent_upload_id" }),
			mock.Anything,
		).Return(nil, gorm.ErrRecordNotFound).Once()

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

		requestService := coreTesting.GetMockRequestService(ctx)
		require.NotNil(tb, requestService)

		// Create a test Request
		request := &models.Request{
			Operation: core.TUSUploadOperationName("test"),
		}
		err := ctx.DB().Create(request).Error
		require.NoError(tb, err)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
			RequestID:   request.ID,
		}
		err = ctx.DB().Create(tusRequest).Error
		require.NoError(tb, err)

		mockProto := coreTesting.NewMockStorageProtocol("test")
		opName := core.TUSUploadOperationName("test")

		// DeleteUpload calls UploadExists internally
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "test_upload_id" }),
			mock.MatchedBy(func(f core.RequestFilter) bool { return f.Operation != nil && *f.Operation == opName }),
		).Return(request, nil).Once()

		requestService.EXPECT().GetRequestData(
			mock.Anything,
			request,
		).Return(tusRequest, nil).Once()

		// Then calls DeleteRequest
		requestService.EXPECT().DeleteRequest(
			mock.Anything,
			request.ID,
		).Return(nil).Once()

		// Test delete upload
		err = tusService.DeleteUpload(ctx, mockProto, "test_upload_id")
		assert.NoError(tb, err)

		// Test upload does not exist
		requestService.EXPECT().QueryRequestData(
			mock.Anything,
			mock.MatchedBy(func(q *models.TUSRequest) bool { return q.TUSUploadID == "nonexistent_upload_id" }),
			mock.Anything,
		).Return(nil, gorm.ErrRecordNotFound).Once()

		err = tusService.DeleteUpload(ctx, mockProto, "nonexistent_upload_id")
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrUploadNotFound, err)

	}, coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}
