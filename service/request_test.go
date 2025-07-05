package service

import (
	"context"
	"errors"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestServiceDefault_CreateRequest(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		protocol := "test"
		// Create test protocol that implements ProtocolPinHandler
		testProto := coreTesting.NewMockProtocol(t, protocol)
		core.RegisterProtocol(protocol, testProto)

		// Define a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}

		// Create the request
		createdReq, err := requestService.CreateRequest(context.Background(), req, nil)
		require.NoError(tb, err)
		assert.NotNil(tb, createdReq)
		assert.Equal(tb, req.Operation, createdReq.Operation)
		assert.Equal(tb, req.Status, createdReq.Status)
		assert.Equal(tb, req.UserID, createdReq.UserID)

		// Verify that the request exists in the database
		var dbReq models.Request
		err = ctx.DB().First(&dbReq, createdReq.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, req.Operation, dbReq.Operation)
		assert.Equal(tb, req.Status, dbReq.Status)
		assert.Equal(tb, req.UserID, dbReq.UserID)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_GetRequest(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		protocol := "test"
		// Create test protocol that implements ProtocolPinHandler
		testProto := coreTesting.NewMockProtocol(t, protocol)
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Get the request
		retrievedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.NotNil(tb, retrievedReq)
		assert.Equal(tb, req.Operation, retrievedReq.Operation)
		assert.Equal(tb, req.Status, retrievedReq.Status)
		assert.Equal(tb, req.UserID, retrievedReq.UserID)

		// 3. Test with non-existent request
		_, err = requestService.GetRequest(context.Background(), 999)
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_UpdateRequestStatus(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Update the request status
		newStatus := models.RequestStatusProcessing
		err = requestService.UpdateRequestStatus(context.Background(), req.ID, newStatus)
		require.NoError(tb, err)

		// 3. Verify the status is updated
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, newStatus, updatedReq.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_CompleteRequest(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Complete the request
		err = requestService.CompleteRequest(context.Background(), req.ID)
		require.NoError(tb, err)

		// 3. Verify the status is updated
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, updatedReq.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_FailRequest(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Fail the request
		failReason := "test failure reason"
		err = requestService.FailRequest(context.Background(), req.ID, failReason)
		require.NoError(tb, err)

		// 3. Verify the status and reason are updated
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, updatedReq.Status)
		assert.Equal(tb, failReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_GetRequestStatus(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		protocol := "test"

		// Create test protocol that implements ProtocolPinHandler
		testProto := coreTesting.NewMockProtocol(t, protocol)
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Get the request status
		status, err := requestService.GetRequestStatus(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.NotNil(tb, status)
		assert.Equal(tb, string(req.Status), status.State)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}

func TestRequestServiceDefault_RequestExists(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Create a test request
		req := &models.Request{
			Operation: "test.operation",
			Status:    models.RequestStatusPending,
			UserID:    lo.ToPtr(uint(1)),
		}
		err := ctx.DB().Create(req).Error
		require.NoError(tb, err)

		// 2. Check if the request exists
		exists, err := requestService.RequestExists(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.True(tb, exists)

		// 3. Check for non-existent request
		exists, err = requestService.RequestExists(context.Background(), 999)
		require.NoError(tb, err)
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService))
}
