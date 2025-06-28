package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	dbMocks "go.lumeweb.com/portal/db/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// testWorkflowRegister registers a workflow with the given name and steps.
func testWorkflowRegister(tb coreTesting.TB, workflowService core.WorkflowService, name string, steps []core.OperationStep) {
	err := workflowService.RegisterWorkflow(name, steps)
	assert.NoError(tb, err)
}

// testWorkflowStart starts a workflow with the given name and initial data.
func testWorkflowStart(tb coreTesting.TB, ctx context.Context, workflowService core.WorkflowService, name string, initialData interface{}) *models.Request {
	var req *models.Request
	var err error

	switch v := initialData.(type) {
	case map[string]interface{}:
		req, err = workflowService.StartWorkflow(ctx, name, v)
	case *models.Request:
		// Convert the *models.Request to a map[string]interface{}
		data := map[string]interface{}{
			"protocol":          v.Protocol,
			"userID":            v.UserID,
			"sourceIP":          v.SourceIP,
			"hash":              v.Hash,
			"cidType":           v.CIDType,
			"uploadHash":        v.UploadHash,
			"uploadHashCIDType": v.UploadHashCIDType,
			"size":              v.Size,
			"mimeType":          v.MimeType,
			"operation":         v.Operation,
			"metadata":          v.Metadata,
		}
		req, err = workflowService.StartWorkflow(ctx, name, data)
	default:
		assert.Fail(tb, "Invalid type for initialData. Must be map[string]interface{} or *models.Request")
		return nil
	}

	assert.NoError(tb, err)
	assert.NotNil(tb, req)
	return req
}

func TestWorkflowCoordinatorDefault_RegisterWorkflow(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "testWorkflow"
		steps := []core.OperationStep{
			{
				Operation: "testOp",
				Handler:   coreMocks.NewMockOperationHandler(tb),
			},
		}

		// Act
		err := workflowService.RegisterWorkflow(workflowName, steps)

		// Assert
		assert.NoError(tb, err)

		_, err = workflowService.GetWorkflow(workflowName)
		assert.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_RegisterWorkflow_Duplicate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "testWorkflow"
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		steps := []core.OperationStep{
			{
				Operation: "testOp",
				Handler:   mockHandler,
			},
		}

		// Act
		err := workflowService.RegisterWorkflow(workflowName, steps)
		assert.NoError(tb, err)

		err = workflowService.RegisterWorkflow(workflowName, steps)

		// Assert
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_GetWorkflow_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		// Act
		_, err := workflowService.GetWorkflow("nonExistentWorkflow")

		// Assert
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_ListWorkflows(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		// Create mock operation handlers
		mockHandler1 := coreMocks.NewMockOperationHandler(tb)
		mockHandler2 := coreMocks.NewMockOperationHandler(tb)

		workflowName1 := "testWorkflow1"
		steps1 := []core.OperationStep{{
			Operation: "testOp1",
			Handler:   mockHandler1,
		}}
		testWorkflowRegister(tb, workflowService, workflowName1, steps1)

		workflowName2 := "testWorkflow2"
		steps2 := []core.OperationStep{{
			Operation: "testOp2",
			Handler:   mockHandler2,
		}}
		testWorkflowRegister(tb, workflowService, workflowName2, steps2)

		// Act
		workflows := workflowService.ListWorkflows()

		// Assert
		assert.Contains(tb, workflows, workflowName1)
		assert.Contains(tb, workflows, workflowName2)
	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_StartWorkflow(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"
		initialData := map[string]interface{}{"key": "value"}

		// Mock operation handler validation
		mockHandler := coreMocks.NewMockOperationHandler(t)
		steps := []core.OperationStep{
			{
				Operation: operationName,
				Handler:   mockHandler,
			},
		}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		// Mock handler validation
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)

		// Mock CreateRequest to return the created request
		mockRequest := &models.Request{
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(mockRequest, nil)

		// Act
		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Assert
		assert.Equal(tb, operationName, req.Operation)

		var metadata WorkflowMetadata
		err := json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)

		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, 0, metadata.CurrentStep)
		assert.Equal(tb, 1, metadata.TotalSteps)
		assert.Equal(tb, initialData, metadata.InitialData)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_StartWorkflow_NoSteps(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "testWorkflow"
		steps := []core.OperationStep{}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		initialData := map[string]interface{}{"key": "value"}

		// Act
		_, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)

		// Assert
		assert.Error(tb, err)
		assert.Equal(tb, "workflow has no steps", err.Error())
	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_CompleteWorkflowStep(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName1 := "testOp1"
		operationName2 := "testOp2"
		initialData := map[string]interface{}{"key": "value"}

		// Create mock operation handlers
		mockHandler1 := coreMocks.NewMockOperationHandler(tb)
		mockHandler2 := coreMocks.NewMockOperationHandler(tb)

		steps := []core.OperationStep{
			{
				Operation: operationName1,
				Handler:   mockHandler1,
			},
			{
				Operation: operationName2,
				Handler:   mockHandler2,
			},
		}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		// Mock StartWorkflow calls
		mockHandler1.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		mockRequest := &models.Request{
			Model: gorm.Model{
				ID: 1, // Mock Request ID
			},
			Operation: operationName1,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":2,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(mockRequest, nil)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Mock GetRequest and CompleteRequest for CompleteWorkflowStep
		requestService.EXPECT().GetRequest(mock.Anything, req.ID).Return(req, nil)
		requestService.EXPECT().CompleteRequest(mock.Anything, req.ID).Return(nil)

		// Mock the next request
		nextRequest := &models.Request{
			Model: gorm.Model{
				ID: 2, // Mock Request ID
			},
			Operation: operationName2,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":1,"total_steps":2,"started_at":1751145351,"prev_request_id":1}`)),
		}

		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(nextRequest, nil)

		requestService.EXPECT().GetRequest(mock.Anything, mock.Anything).Return(nextRequest, nil)
		requestService.EXPECT().CompleteRequest(mock.Anything, mock.Anything).Return(nil)

		// Act
		err := workflowService.CompleteWorkflowStep(context.Background(), req.ID)
		assert.NoError(tb, err)

		// Complete the last step
		err = workflowService.CompleteWorkflowStep(context.Background(), nextRequest.ID)

		// Assert
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_CompleteWorkflowStep_InvalidMeta(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// Create a request with invalid metadata
		req := &models.Request{
			Operation: "testOp",
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte("invalid json")),
		}

		// Mock GetRequest call
		requestService.EXPECT().GetRequest(mock.Anything, req.ID).Return(req, nil)

		// Act
		err := workflowService.CompleteWorkflowStep(context.Background(), req.ID)

		// Assert
		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "invalid workflow meta")

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_FailWorkflowStep(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"
		initialData := map[string]interface{}{"key": "value"}
		failureReason := "test failure"

		// Mock operation handler
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		steps := []core.OperationStep{{
			Operation:       operationName,
			FailureBehavior: core.FailWorkflow,
			Handler:         mockHandler,
		}}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		// Mock handler validation
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)

		// Mock CreateRequest to return the created request
		mockRequest := &models.Request{
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(mockRequest, nil)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Mock the UpdateRequest call in FailWorkflowStep
		mockRequest.Status = models.RequestStatusFailed
		mockRequest.StatusMessage = failureReason

		requestService.EXPECT().GetRequest(mock.Anything, req.ID).Return(req, nil)
		requestService.EXPECT().FailRequest(mock.Anything, req.ID, mock.Anything).Return(nil)

		// Act
		err := workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)

		// Assert
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_GetWorkflowStatus(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"
		initialData := map[string]interface{}{"key": "value"}

		// Mock operation handler
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		steps := []core.OperationStep{{
			Operation: operationName,
			Handler:   mockHandler,
		}}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		requestService.EXPECT().RegisterRequestModel(operationName, mock.Anything).Return()

		// Mock CreateRequest to return the created request
		mockRequest := &models.Request{
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}

		// Mock the CreateRequest method
		mockRequestDataModel := dbMocks.NewMockRequestDataModel(t)
		requestService.RegisterRequestModel(operationName, mockRequestDataModel)
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).Return(mockRequest, nil)
		requestService.EXPECT().GetRequest(mock.Anything, mockRequest.ID).Return(mockRequest, nil)
		requestService.EXPECT().CompleteRequest(mock.Anything, mockRequest.ID).Return(nil)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Act
		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)

		// Assert
		assert.NoError(tb, err)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, 0, status.CurrentStep)
		assert.Equal(tb, 1, status.TotalSteps)
		assert.Equal(tb, string(models.RequestStatusPending), status.Status)
		assert.Equal(tb, req.ID, status.CurrentStepID)
		assert.Equal(tb, 0.0, status.Progress)

		// Complete the step
		err = requestService.CompleteRequest(context.Background(), req.ID)
		require.NoError(tb, err)

		mockRequest.Status = models.RequestStatusCompleted

		status, err = workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, 100.0, status.Progress)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_FailWorkflowStep_RetryStep_UpdatesStatus(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"
		initialData := map[string]interface{}{"key": "value"}

		// Mock operation handler
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		steps := []core.OperationStep{{
			Operation:       operationName,
			FailureBehavior: core.RetryStep,
			Handler:         mockHandler,
		}}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		// Mock CreateRequest to return the created request
		mockRequest := &models.Request{
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}

		requestService.EXPECT().RegisterRequestModel(operationName, mock.Anything).Return()
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).Return(mockRequest, nil)

		// Mock the CreateRequest method
		mockRequestDataModel := dbMocks.NewMockRequestDataModel(t)
		requestService.RegisterRequestModel(operationName, mockRequestDataModel)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Mock GetRequest call
		requestService.EXPECT().GetRequest(mock.Anything, req.ID).Return(req, nil)

		// Mock UpdateRequest call
		requestService.EXPECT().FailRequest(mock.Anything, req.ID, mock.Anything).Return(nil)

		// Act
		// Fail the step with RetryStep behavior
		err := workflowService.FailWorkflowStep(context.Background(), req.ID, "test failure")

		// Assert
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowMetadata_JSONMarshal(t *testing.T) {
	// Arrange
	metadata := WorkflowMetadata{
		WorkflowName:  "testWorkflow",
		CurrentStep:   1,
		TotalSteps:    3,
		NextRequestID: 42,
		PrevRequestID: 1,
		StartedAt:     time.Now().Unix(),
		InitialData:   map[string]interface{}{"key": "value"},
	}

	// Act
	jsonBytes, err := json.Marshal(metadata)

	// Assert
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)

	var unmarshaledMetadata WorkflowMetadata
	err = json.Unmarshal(jsonBytes, &unmarshaledMetadata)
	assert.NoError(t, err)
	assert.Equal(t, metadata, unmarshaledMetadata)
}

func TestTUSUploadStep(t *testing.T) {
	// Act
	step := TUSUploadStep(core.FailWorkflow)

	// Assert
	assert.Equal(t, models.RequestOperationTusUpload, step.Operation)
	assert.Equal(t, core.FailWorkflow, step.FailureBehavior)
}

func TestNewWorkflowCoordinator(t *testing.T) {
	// Act
	coordinator, opts, err := NewWorkflowCoordinator()

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, coordinator)
	assert.NotEmpty(t, opts)
}

func TestWorkflowCoordinatorDefault_StartWorkflow_InitialDataIsRequest(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"

		// Mock operation handler
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		steps := []core.OperationStep{{
			Operation: operationName,
			Handler:   mockHandler,
		}}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		testData := []byte("test_data_for_hashing")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		// Create an initial request
		initialRequest := &models.Request{
			Protocol:          "testProtocol",
			UserID:            123,
			SourceIP:          "127.0.0.1",
			Hash:              mh,
			CIDType:           1,
			UploadHash:        mh,
			UploadHashCIDType: 2,
			Size:              1024,
			MimeType:          "testMimeType",
			Operation:         operationName,
			Metadata:          datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351}`)),
		}

		requestService.EXPECT().RegisterRequestModel(operationName, mock.Anything).Return()

		// Mock the CreateRequest method
		mockRequestDataModel := dbMocks.NewMockRequestDataModel(t)
		requestService.RegisterRequestModel(operationName, mockRequestDataModel)

		// Mock CreateRequest to return the created request with the initial request's fields
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).Return(initialRequest, nil)

		// Act
		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialRequest)

		// Assert
		assert.Equal(tb, operationName, req.Operation)
		assert.Equal(tb, "testProtocol", req.Protocol)
		assert.Equal(tb, uint(123), req.UserID)
		assert.Equal(tb, "127.0.0.1", req.SourceIP)
		assert.Equal(tb, mh, req.Hash)
		assert.Equal(tb, uint64(1), req.CIDType)
		assert.Equal(tb, mh, req.UploadHash)
		assert.Equal(tb, uint64(2), req.UploadHashCIDType)
		assert.Equal(tb, uint64(1024), req.Size)
		assert.Equal(tb, "testMimeType", req.MimeType)

		var metadata WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)

		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, 0, metadata.CurrentStep)
		assert.Equal(tb, 1, metadata.TotalSteps)
		assert.Nil(tb, metadata.InitialData) // InitialData should be nil

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_FailWorkflowStep_ContinueWorkflow(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName1 := "testOp1"
		operationName2 := "testOp2"
		initialData := map[string]interface{}{"key": "value"}
		failureReason := "test failure"

		// Mock operation handlers
		mockHandler1 := coreMocks.NewMockOperationHandler(tb)
		mockHandler2 := coreMocks.NewMockOperationHandler(tb)
		mockHandler1.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		steps := []core.OperationStep{
			{
				Operation:       operationName1,
				FailureBehavior: core.ContinueWorkflow,
				Handler:         mockHandler1,
			},
			{
				Operation: operationName2,
				Handler:   mockHandler2,
			},
		}

		requestService.EXPECT().RegisterRequestModel(operationName1, mock.Anything).Return()
		requestService.EXPECT().RegisterRequestModel(operationName2, mock.Anything).Return()
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		// Mock the CreateRequest method
		mockRequestDataModel1 := dbMocks.NewMockRequestDataModel(t)
		mockRequestDataModel2 := dbMocks.NewMockRequestDataModel(t)
		requestService.RegisterRequestModel(operationName1, mockRequestDataModel1)
		requestService.RegisterRequestModel(operationName2, mockRequestDataModel2)

		// Mock StartWorkflow to return the created request
		mockRequest := &models.Request{
			Model: gorm.Model{
				ID: 1, // Mock Request ID
			},
			Operation: operationName1,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":2,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(mockRequest, nil)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Mock the next request
		nextRequest := &models.Request{
			Model: gorm.Model{
				ID: 2, // Mock Request ID
			},
			Operation: operationName2,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":1,"total_steps":2,"started_at":1751145351,"prev_request_id":1}`)),
		}

		requestService.EXPECT().GetRequest(mock.Anything, mock.Anything).Return(nextRequest, nil)
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(nextRequest, nil)

		requestService.EXPECT().FailRequest(mock.Anything, nextRequest.ID, failureReason).Return(nil)

		// Act
		err := workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)

		// Assert
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}

func TestWorkflowCoordinatorDefault_FailWorkflowStep_RetryStep(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[*coreMocks.MockRequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "testWorkflow"
		operationName := "testOp"
		initialData := map[string]interface{}{"key": "value"}
		failureReason := "test failure"

		// Mock operation handler
		mockHandler := coreMocks.NewMockOperationHandler(tb)
		mockHandler.EXPECT().ValidateRequest(mock.Anything, mock.Anything).Return(nil)
		steps := []core.OperationStep{{
			Operation:       operationName,
			FailureBehavior: core.RetryStep,
			Handler:         mockHandler,
		}}
		testWorkflowRegister(tb, workflowService, workflowName, steps)

		requestService.EXPECT().RegisterRequestModel(operationName, mock.Anything).Return()

		// Mock the CreateRequest method
		mockRequestDataModel := dbMocks.NewMockRequestDataModel(t)
		requestService.RegisterRequestModel(operationName, mockRequestDataModel)

		// Mock StartWorkflow to return the created request
		mockRequest := &models.Request{
			Model: gorm.Model{
				ID: 1, // Mock Request ID
			},
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"workflow_name":"testWorkflow","current_step":0,"total_steps":1,"started_at":1751145351,"initial_data":{"key":"value"}}`)),
		}
		requestService.EXPECT().CreateRequest(mock.Anything, mock.Anything, mock.Anything).
			Return(mockRequest, nil)
		requestService.EXPECT().GetRequest(mock.Anything, mockRequest.ID).Return(mockRequest, nil)

		req := testWorkflowStart(tb, context.Background(), workflowService, workflowName, initialData)

		// Mock the UpdateRequest call in FailWorkflowStep
		mockRequest.Status = models.RequestStatusPending

		requestService.EXPECT().FailRequest(mock.Anything, req.ID, failureReason).Return(nil)

		// Act
		err := workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)

		// Assert
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator))
}
