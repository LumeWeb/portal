package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"testing"
)

func registerTestProtocol(tb coreTesting.TB) {
	protocolName := "test"
	testProto := coreTesting.NewMockProtocol(tb, protocolName)

	// Register test operations
	op1 := coreTesting.NewMockOperation(tb).WithType(func() string { return "test.op1" })
	op2 := coreTesting.NewMockOperation(tb).WithType(func() string { return "test.op2" })
	op3 := coreTesting.NewMockOperation(tb).WithType(func() string { return "test.op3" })
	op4 := coreTesting.NewMockOperation(tb).WithType(func() string { return "test.operation" })

	testProto.WithOperation(op1)
	testProto.WithOperation(op2)
	testProto.WithOperation(op3)
	testProto.WithOperation(op4)

	core.RegisterProtocol(protocolName, testProto)
}

func TestWorkflowCoordinatorDefault_RegisterWorkflow_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		steps := []core.OperationStep{
			{
				Operation: operationName,
				Handler:   mockHandler,
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Verify the workflow is registered
		wf, err := workflowService.GetWorkflow(workflowName)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, wf.Name)
		assert.Len(tb, wf.Steps, 1)
		assert.Equal(tb, operationName, wf.Steps[0].Operation)

		// 4. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)

		// 5. Verify the request is in the database
		var dbReq models.Request
		err = ctx.DB().First(&dbReq, req.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, operationName, dbReq.Operation)

		// 6. Get the workflow status
		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.NotNil(tb, status)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, 0, status.CurrentStep)
		assert.Equal(tb, 1, status.TotalSteps)
		assert.Equal(tb, string(models.RequestStatusProcessing), status.Status)

		// 7. Complete the workflow step
		err = workflowService.CompleteWorkflowStep(context.Background(), req.ID)
		require.NoError(tb, err)

		// 8. Verify the request status is updated
		updatedStatus, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.RequestStatusCompleted), updatedStatus.Status)
		assert.Equal(tb, 100.0, updatedStatus.Progress)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_FailWorkflowStep_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"
		failureReason := "test failure"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.FailWorkflow,
				Handler:         mockHandler,
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)

		// 4. Fail the workflow step
		err = workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)
		require.NoError(tb, err)

		// 5. Verify the request status is updated
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, updatedReq.Status)
		assert.Equal(tb, failureReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_ContinueWorkflowStep_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow with two steps
		workflowName := "testWorkflow"
		operationName1 := "test.op1"
		operationName2 := "test.op2"
		failureReason := "test failure"

		// Create mock operation handlers
		mockHandler1 := coreTesting.NewMockOperationHandler(tb)
		mockHandler2 := coreTesting.NewMockOperationHandler(tb)

		// Create mock operations
		mockOperation1 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName1 }).WithHandler(func() core.OperationHandler { return mockHandler1 })
		mockOperation2 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName2 }).WithHandler(func() core.OperationHandler { return mockHandler2 })

		steps := []core.OperationStep{
			{
				Operation:       operationName1,
				FailureBehavior: core.ContinueWorkflow,
				Handler:         mockOperation1.Handler(),
			},
			{
				Operation: operationName2,
				Handler:   mockOperation2.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName1, req.Operation)

		// 4. Fail the first workflow step with ContinueWorkflow behavior
		err = workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)
		require.NoError(tb, err)

		// 5. Verify that a new request for the second step is created
		var nextReq models.Request
		err = ctx.DB().Where("operation = ? AND status = ?", operationName2, models.RequestStatusPending).First(&nextReq).Error
		require.NoError(tb, err)

		// 6. Verify the metadata of the new request
		var metadata WorkflowMetadata
		err = json.Unmarshal(nextReq.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, 1, metadata.CurrentStep)
		assert.Equal(tb, 2, metadata.TotalSteps)
		assert.Equal(tb, req.ID, metadata.PrevRequestID)

		// 7. Verify the status of the first request is failed
		failedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, failedReq.Status)
		assert.Equal(tb, failureReason, failedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_RetryWorkflowStep_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"
		failureReason := "test failure"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		// Create a mock operation
		mockOperation := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName }).WithHandler(func() core.OperationHandler { return mockHandler })

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.RetryStep,
				Handler:         mockOperation.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)

		// 4. Fail the workflow step with RetryStep behavior
		err = workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)
		require.NoError(tb, err)

		// 5. Verify the request status is updated to pending
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusPending, updatedReq.Status)
		assert.Equal(tb, failureReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_InitialDataIsRequest_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		// Create a mock operation
		mockOperation := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName }).WithHandler(func() core.OperationHandler { return mockHandler })

		steps := []core.OperationStep{
			{
				Operation: operationName,
				Handler:   mockOperation.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Create an initial request
		initialRequest := &models.Request{
			Protocol:  "testProtocol",
			UserID:    123,
			SourceIP:  "127.0.0.1",
			Operation: operationName,
		}

		err = ctx.DB().Create(initialRequest).Error
		require.NoError(tb, err)

		// 4. Start the workflow with the initial request as data
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialRequest)
		require.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)
		assert.Equal(tb, "testProtocol", req.Protocol)
		assert.Equal(tb, uint(123), req.UserID)
		assert.Equal(tb, "127.0.0.1", req.SourceIP)

		// 5. Verify the request is in the database
		var dbReq models.Request
		err = ctx.DB().First(&dbReq, req.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, operationName, dbReq.Operation)
		assert.Equal(tb, "testProtocol", dbReq.Protocol)
		assert.Equal(tb, uint(123), dbReq.UserID)
		assert.Equal(tb, "127.0.0.1", dbReq.SourceIP)

		// 6. Get the workflow status
		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.NotNil(tb, status)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, 0, status.CurrentStep)
		assert.Equal(tb, 1, status.TotalSteps)
		assert.Equal(tb, string(models.RequestStatusPending), status.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_GetWorkflowStatus_PreviousSteps_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow with three steps
		workflowName := "testWorkflow"
		operationName1 := "test.op1"
		operationName2 := "test.op2"
		operationName3 := "test.op3"

		// Create mock operation handlers
		mockHandler1 := coreTesting.NewMockOperationHandler(tb)
		mockHandler2 := coreTesting.NewMockOperationHandler(tb)
		mockHandler3 := coreTesting.NewMockOperationHandler(tb)

		// Create mock operations
		mockOperation1 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName1 }).WithHandler(func() core.OperationHandler { return mockHandler1 })
		mockOperation2 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName2 }).WithHandler(func() core.OperationHandler { return mockHandler2 })
		mockOperation3 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName3 }).WithHandler(func() core.OperationHandler { return mockHandler3 })

		steps := []core.OperationStep{
			{
				Operation: operationName1,
				Handler:   mockOperation1.Handler(),
			},
			{
				Operation: operationName2,
				Handler:   mockOperation2.Handler(),
			},
			{
				Operation: operationName3,
				Handler:   mockOperation3.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// 4. Complete the first step
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		require.NoError(tb, err)

		// Get the second request
		var req2 models.Request
		err = ctx.DB().Where("operation = ? AND status = ?", operationName2, models.RequestStatusPending).First(&req2).Error
		require.NoError(tb, err)

		// 5. Complete the second step
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		require.NoError(tb, err)

		// Get the third request
		var req3 models.Request
		err = ctx.DB().Where("operation = ? AND status = ?", operationName3, models.RequestStatusPending).First(&req3).Error
		require.NoError(tb, err)

		// 6. Get the workflow status for the third request
		status, err := workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		require.NoError(tb, err)
		assert.NotNil(tb, status)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, 2, status.CurrentStep)
		assert.Equal(tb, 3, status.TotalSteps)
		assert.Equal(tb, string(models.RequestStatusPending), status.Status)
		assert.Len(tb, status.PreviousSteps, 2)
		assert.Contains(tb, status.PreviousSteps, req1.ID)
		assert.Contains(tb, status.PreviousSteps, req2.ID)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_ExecuteWorkflowStep_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"

		// Create a mock operation handler that will succeed
		mockHandler := coreTesting.NewMockOperationHandler(tb).WithExecute(
			func(ctx context.Context, req *models.Request) error {
				return nil
			},
		)

		steps := []core.OperationStep{
			{
				Operation: operationName,
				Handler:   mockHandler,
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// 4. Execute the workflow step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req.ID)
		require.NoError(tb, err)

		// 5. Verify the request was completed
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, updatedReq.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_ExecuteWorkflowStep_Failure_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"
		failureReason := "test failure"

		// Create a mock operation handler that will fail
		mockHandler := coreTesting.NewMockOperationHandler(tb).WithExecute(
			func(ctx context.Context, req *models.Request) error {
				return fmt.Errorf("%s", failureReason)
			},
		)

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.FailWorkflow,
				Handler:         mockHandler,
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// 4. Execute the workflow step (should fail)
		err = workflowService.ExecuteWorkflowStep(context.Background(), req.ID)
		require.NoError(tb, err) // The error is handled internally

		// 5. Verify the request was failed
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, updatedReq.Status)
		assert.Equal(tb, failureReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_CanTransition_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		// Create a mock operation
		mockOperation := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName }).WithHandler(func() core.OperationHandler { return mockHandler })

		steps := []core.OperationStep{
			{
				Operation: operationName,
				Handler:   mockOperation.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// 4. Test CanTransition with pending request
		canTransition, err := workflowService.CanTransition(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.True(tb, canTransition)

		// 5. Complete the request
		err = workflowService.CompleteWorkflowStep(context.Background(), req.ID)
		require.NoError(tb, err)

		// 6. Test CanTransition with completed request
		canTransition, err = workflowService.CanTransition(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.False(tb, canTransition)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_GetWorkflowStepInfo_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow
		workflowName := "testWorkflow"
		operationName := "test.operation"

		// Create a mock operation handler
		mockHandler := coreTesting.NewMockOperationHandler(tb)

		// Create a mock operation
		mockOperation := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName }).WithHandler(func() core.OperationHandler { return mockHandler })

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.FailWorkflow,
				Handler:         mockOperation.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// 4. Get workflow step info
		info, err := workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
		require.NoError(tb, err)
		assert.Equal(tb, operationName, info.Operation)
		assert.Equal(tb, core.FailWorkflow, info.FailureBehavior)
		assert.Equal(tb, string(models.RequestStatusPending), info.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}

func TestWorkflowCoordinatorDefault_CompleteWorkflowStep_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registerTestProtocol(tb)

		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		// 1. Define a workflow with two steps
		workflowName := "testWorkflow"
		operationName1 := "test.op1"
		operationName2 := "test.op2"

		// Create mock operation handlers
		mockHandler1 := coreTesting.NewMockOperationHandler(tb)
		mockHandler2 := coreTesting.NewMockOperationHandler(tb)

		// Create mock operations
		mockOperation1 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName1 }).WithHandler(func() core.OperationHandler { return mockHandler1 })
		mockOperation2 := coreTesting.NewMockOperation(tb).WithType(func() string { return operationName2 }).WithHandler(func() core.OperationHandler { return mockHandler2 })

		steps := []core.OperationStep{
			{
				Operation: operationName1,
				Handler:   mockOperation1.Handler(),
			},
			{
				Operation: operationName2,
				Handler:   mockOperation2.Handler(),
			},
		}

		// 2. Register the workflow
		err := workflowService.RegisterWorkflow(workflowName, steps)
		require.NoError(tb, err)

		// 3. Start the workflow
		initialData := map[string]interface{}{"key": "value"}
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, initialData)
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// 4. Complete the first step
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		require.NoError(tb, err)

		// 5. Verify that a new request for the second step is created
		var req2 models.Request
		err = ctx.DB().Where("operation = ? AND status = ?", operationName2, models.RequestStatusPending).First(&req2).Error
		require.NoError(tb, err)

		// 6. Verify the metadata of the new request
		var metadata WorkflowMetadata
		err = json.Unmarshal(req2.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, 1, metadata.CurrentStep)
		assert.Equal(tb, 2, metadata.TotalSteps)
		assert.Equal(tb, req1.ID, metadata.PrevRequestID)

		// 7. Complete the second step
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		require.NoError(tb, err)

		// 8. Verify that the second request is completed
		updatedReq2, err := requestService.GetRequest(context.Background(), req2.ID)
		require.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, updatedReq2.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
	)
}
