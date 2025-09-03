package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multiformats/go-multihash"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/datatypes"
)

// Test 1: Workflow Registration Tests
func TestWorkflowRegistration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "registrationTestWorkflow"
		operationName := "test.operation.registration"
		steps := []core.OperationStep{
			{
				Operation: operationName,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		assert.NoError(tb, err)

		// Verify workflow is registered
		wf, err := workflowService.GetWorkflow(workflowName)
		assert.NoError(tb, err)
		assert.Equal(tb, workflowName, wf.Name)
		assert.Len(tb, wf.Steps, 1)
		assert.Equal(tb, operationName, wf.Steps[0].Operation)

		// Try to register duplicate workflow (should fail)
		err = workflowService.RegisterWorkflow(workflowName, steps, false)
		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "already exists")

		// Try to register workflow with empty steps (should fail)
		emptyWorkflowName := "emptyStepsWorkflow"
		emptySteps := []core.OperationStep{}
		err = workflowService.RegisterWorkflow(emptyWorkflowName, emptySteps, false)
		assert.Error(tb, err)
		assert.Equal(tb, core.ErrWorkflowHasNoSteps, err)

		// List all registered workflows
		workflows := workflowService.ListWorkflows()
		assert.Contains(tb, workflows, workflowName)
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.registration"),
	)
}

// Test 2: Workflow Start Tests
func TestWorkflowStart(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "startTestWorkflow"
		operationName := "test.operation.start"
		initialData := map[string]interface{}{"key": "value", "number": 42}

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow with map data
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(initialData))
		assert.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)

		// Verify metadata
		var metadata WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, "step-test.operation.start-0", metadata.CurrentStepID)
		assert.Equal(tb, 1, metadata.TotalSteps)
		assert.Equal(tb, uint(0), metadata.PrevRequestID)
		assert.Equal(tb, uint(0), metadata.NextRequestID)

		// Verify data was stored correctly
		var storedData map[string]interface{}
		err = json.Unmarshal(metadata.Data, &storedData)
		require.NoError(tb, err)
		// Compare marshaled JSON to handle number type differences
		expectedJSON, err := json.Marshal(initialData)
		require.NoError(tb, err)
		actualJSON, err := json.Marshal(storedData)
		require.NoError(tb, err)
		assert.JSONEq(tb, string(expectedJSON), string(actualJSON))
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.start"),
	)
}

// Test 3: Single Step Workflow Completion Tests
func TestSingleStepWorkflowCompletion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "singleStepWorkflow"
		operationName := "test.operation.single"
		initialData := map[string]interface{}{"test": "data"}

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(initialData))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Execute the step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req.ID)
		assert.NoError(tb, err)

		// Complete the step
		err = workflowService.CompleteWorkflowStep(context.Background(), req.ID)
		assert.NoError(tb, err)

		// Verify workflow is completed and cleaned up
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		assert.Error(tb, err) // Should be deleted
		assert.Nil(tb, updatedReq)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.single"),
	)
}

// Test 4: Multi-Step Workflow Transition Tests
func TestMultiStepWorkflowTransition(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "multiStepWorkflow"
		operationName1 := "test.operation.step1"
		operationName2 := "test.operation.step2"
		operationName3 := "test.operation.step3"
		initialData := map[string]interface{}{"workflow": "test"}

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
			{
				Operation:  operationName3,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(initialData))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// Execute first step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Complete first step - should create second request
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Find second request
		var req2 models.Request
		err = ctx.DB().Model(&req2).Where("operation = ? AND status = ?", operationName2, models.RequestStatusProcessing).First(&req2).Error
		require.NoError(tb, err)
		assert.NotEqual(tb, req1.ID, req2.ID)

		// Verify metadata chaining
		var metadata2 WorkflowMetadata
		err = json.Unmarshal(req2.Metadata, &metadata2)
		require.NoError(tb, err)
		assert.Equal(tb, req1.ID, metadata2.PrevRequestID)
		assert.Equal(tb, workflowName, metadata2.WorkflowName)

		// Execute second step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Complete second step - should create third request
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Find third request
		var req3 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName3, models.RequestStatusProcessing).First(&req3).Error
		require.NoError(tb, err)
		assert.NotEqual(tb, req1.ID, req3.ID)
		assert.NotEqual(tb, req2.ID, req3.ID)

		// Verify metadata chaining
		var metadata3 WorkflowMetadata
		err = json.Unmarshal(req3.Metadata, &metadata3)
		require.NoError(tb, err)
		assert.Equal(tb, req2.ID, metadata3.PrevRequestID)
		assert.Equal(tb, workflowName, metadata3.WorkflowName)

		// Execute third step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Complete third step - should complete workflow
		err = workflowService.CompleteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Verify all requests are cleaned up
		var count int64
		err = ctx.DB().Model(&models.Request{}).Where("id IN (?, ?, ?)", req1.ID, req2.ID, req3.ID).Count(&count).Error
		require.NoError(tb, err)
		assert.Equal(tb, int64(0), count)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.step1", "test.operation.step2", "test.operation.step3"),
	)
}

// Test 5: Fail Workflow Behavior Tests
func TestFailWorkflowBehavior(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "failWorkflowTest"
		operationName := "test.operation.fail"
		failureReason := "test failure reason"

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.FailWorkflow,
				Foreground:      true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Fail the workflow step
		err = workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)
		assert.NoError(tb, err)

		// Verify request is marked as failed
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, updatedReq.Status)
		assert.Equal(tb, failureReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.fail"),
	)
}

// Test 6: Continue Workflow Behavior Tests
func TestContinueWorkflowBehavior(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "continueWorkflowTest"
		operationName1 := "test.operation.continue1"
		operationName2 := "test.operation.continue2"
		failureReason := "test failure but continue"

		steps := []core.OperationStep{
			{
				Operation:       operationName1,
				FailureBehavior: core.ContinueWorkflow,
				Foreground:      true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// Fail first step with ContinueWorkflow behavior
		err = workflowService.FailWorkflowStep(context.Background(), req1.ID, failureReason)
		assert.NoError(tb, err)

		// Verify first request is marked as failed
		failedReq, err := requestService.GetRequest(context.Background(), req1.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusFailed, failedReq.Status)
		assert.Equal(tb, failureReason, failedReq.StatusMessage)

		// Verify second request was created
		var req2 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName2, models.RequestStatusProcessing).First(&req2).Error
		require.NoError(tb, err)
		assert.NotEqual(tb, req1.ID, req2.ID)

		// Verify metadata chaining
		var metadata WorkflowMetadata
		err = json.Unmarshal(req2.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, req1.ID, metadata.PrevRequestID)
		assert.Equal(tb, "step-test.operation.continue2-1", metadata.CurrentStepID)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.continue1", "test.operation.continue2"),
	)
}

// Test 7: Retry Step Behavior Tests
func TestRetryStepBehavior(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "retryWorkflowTest"
		operationName := "test.operation.retry"
		failureReason := "test retry reason"

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.RetryStep,
				Foreground:      true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Fail step with RetryStep behavior
		err = workflowService.FailWorkflowStep(context.Background(), req.ID, failureReason)
		assert.NoError(tb, err)

		// Verify request status is reset to pending for retry
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusPending, updatedReq.Status)
		assert.Equal(tb, failureReason, updatedReq.StatusMessage)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.retry"),
	)
}

// Test 8: Workflow Status Tests
func TestWorkflowStatus(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "statusTestWorkflow"
		operationName1 := "test.operation.status1"
		operationName2 := "test.operation.status2"
		operationName3 := "test.operation.status3"

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
			{
				Operation:  operationName3,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// Get initial status
		status, err := workflowService.GetWorkflowStatus(context.Background(), req1.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, 0, status.CurrentStep) // First step
		assert.Equal(tb, 3, status.TotalSteps)
		assert.Equal(tb, models.RequestStatusPending, status.Status)
		assert.Equal(tb, req1.ID, status.CurrentStepID)
		assert.Equal(tb, 0.0, status.Progress) // 0% progress

		// Execute first step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Complete first step
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Find second request
		var req2 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName2, models.RequestStatusProcessing).First(&req2).Error
		require.NoError(tb, err)

		// Get status for second step
		status, err = workflowService.GetWorkflowStatus(context.Background(), req2.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, 1, status.CurrentStep) // Second step
		assert.Equal(tb, 3, status.TotalSteps)
		assert.Equal(tb, models.RequestStatusProcessing, status.Status)
		assert.Equal(tb, req2.ID, status.CurrentStepID)

		// Progress should be around 50% (1 step completed + 50% of current step)
		assert.InDelta(tb, 50.0, status.Progress, 1.0)

		// Execute second step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Complete second step
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Find third request
		var req3 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName3, models.RequestStatusProcessing).First(&req3).Error
		require.NoError(tb, err)

		// Get status for third step
		status, err = workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, 2, status.CurrentStep) // Third step
		assert.Equal(tb, 3, status.TotalSteps)
		assert.Equal(tb, models.RequestStatusProcessing, status.Status)
		assert.Equal(tb, req3.ID, status.CurrentStepID)

		// Progress should be around 83% (2 steps completed + 50% of current step)
		assert.InDelta(tb, 83.33, status.Progress, 1.0)

		// Execute third step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Complete third step - workflow should be completed
		err = workflowService.CompleteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Get final status
		status, err = workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, status.Status)
		assert.Equal(tb, 100.0, status.Progress)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.status1", "test.operation.status2", "test.operation.status3"),
	)
}

// Test 9: Workflow Step Info Tests
func TestWorkflowStepInfo(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "stepInfoTestWorkflow"
		operationName := "test.operation.stepinfo"

		steps := []core.OperationStep{
			{
				Operation:       operationName,
				FailureBehavior: core.ContinueWorkflow,
				Foreground:      true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Get step info
		info, err := workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, operationName, info.Operation)
		assert.Equal(tb, core.ContinueWorkflow, info.FailureBehavior)
		assert.Equal(tb, models.RequestStatusPending, info.Status)

		// Execute step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req.ID)
		assert.NoError(tb, err)

		err = workflowService.CompleteWorkflowStep(context.Background(), req.ID)
		assert.NoError(tb, err)

		// Get updated step info
		info, err = workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, operationName, info.Operation)
		assert.Equal(tb, core.ContinueWorkflow, info.FailureBehavior)
		assert.Equal(tb, models.RequestStatusCompleted, info.Status)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.stepinfo"),
	)
}

// Test 10: Workflow Cleanup Tests
func TestWorkflowCleanup(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "cleanupTestWorkflow"
		operationName1 := "test.operation.cleanup1"
		operationName2 := "test.operation.cleanup2"
		operationName3 := "test.operation.cleanup3"

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
			{
				Operation:  operationName3,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// Complete all steps
		err = workflowService.ExecuteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Find second request
		var req2 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName2, models.RequestStatusProcessing).First(&req2).Error
		require.NoError(tb, err)

		err = workflowService.ExecuteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Find third request
		var req3 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName3, models.RequestStatusProcessing).First(&req3).Error
		require.NoError(tb, err)

		err = workflowService.ExecuteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Verify all requests are cleaned up
		var count int64
		err = ctx.DB().Model(&models.Request{}).Where("id IN (?, ?, ?)", req1.ID, req2.ID, req3.ID).Count(&count).Error
		require.NoError(tb, err)
		assert.Equal(tb, int64(0), count)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.cleanup1", "test.operation.cleanup2", "test.operation.cleanup3"),
	)
}

// Test 11: Workflow Disable/Enable Tests
func TestWorkflowDisableEnable(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "disableEnableTestWorkflow"
		operationName := "test.operation.disable"

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Disable workflow
		err = workflowService.DisableWorkflow(workflowName)
		assert.NoError(tb, err)

		// Attempt to start disabled workflow (should return nil without error)
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		assert.NoError(tb, err)
		assert.Nil(tb, req)

		// Enable workflow
		err = workflowService.EnableWorkflow(workflowName)
		assert.NoError(tb, err)

		// Successfully start re-enabled workflow
		req, err = workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		assert.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.disable"),
	)
}

// Test 12: Request to Workflow Conversion Tests
func TestRequestToWorkflowConversion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "conversionTestWorkflow"
		operationName1 := "test.operation.convert1"
		operationName2 := "test.operation.convert2"
		requestOperation := "initial.request.operation"

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Create standalone request
		initialRequest := &models.Request{
			Protocol:  "testProtocol",
			UserID:    lo.ToPtr(uint(123)),
			SourceIP:  "127.0.0.1",
			Operation: requestOperation,
			Status:    models.RequestStatusPending,
		}

		err = ctx.DB().Create(initialRequest).Error
		require.NoError(tb, err)

		// Convert request to workflow instance
		err = workflowService.ConvertRequestToWorkflow(context.Background(), initialRequest.ID, workflowName, 0)
		assert.NoError(tb, err)

		// Verify request was updated
		updatedReq, err := requestService.GetRequest(context.Background(), initialRequest.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, operationName1, updatedReq.Operation) // Should be first step operation
		assert.Equal(tb, "testProtocol", updatedReq.Protocol)
		assert.Equal(tb, lo.ToPtr(uint(123)), updatedReq.UserID)
		assert.Equal(tb, "127.0.0.1", updatedReq.SourceIP)

		// Verify metadata was updated correctly
		var metadata WorkflowMetadata
		err = json.Unmarshal(updatedReq.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, "step-test.operation.convert1-0", metadata.CurrentStepID)
		assert.Equal(tb, 2, metadata.TotalSteps)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.convert1", "test.operation.convert2", "initial.request.operation"),
	)
}

// Test 13: Workflow Instance Discovery Tests
func TestWorkflowInstanceDiscovery(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "discoveryTestWorkflow"
		operationName1 := "test.operation.discovery1"
		operationName2 := "test.operation.discovery2"

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start multiple workflow instances
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"instance": 1}))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		req2, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"instance": 2}))
		require.NoError(tb, err)
		require.NotNil(tb, req2)

		// Complete first step of both workflows
		err = workflowService.ExecuteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		err = workflowService.ExecuteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Find second requests for both workflows
		var req1Step2 models.Request
		jsonQuery1 := datatypes.JSONQuery("metadata").Equals(req1.ID, "prev_request_id")
		err = ctx.DB().Model(&models.Request{}).Where("operation = ?", operationName2).Where(jsonQuery1).First(&req1Step2).Error
		require.NoError(tb, err)

		var req2Step2 models.Request
		jsonQuery2 := datatypes.JSONQuery("metadata").Equals(req2.ID, "prev_request_id")
		err = ctx.DB().Model(&models.Request{}).Where("operation = ?", operationName2).Where(jsonQuery2).First(&req2Step2).Error
		require.NoError(tb, err)

		// Find workflow instances
		filter := core.RequestFilter{
			Limit: 100,
		}
		instances, err := workflowService.FindWorkflowInstances(context.Background(), workflowName, filter)
		assert.NoError(tb, err)
		assert.Len(tb, instances, 2)

		// Verify instances contain the correct final step requests
		instanceIDs := make([]uint, len(instances))
		for i, instance := range instances {
			instanceIDs[i] = instance.Request.ID
		}
		assert.Contains(tb, instanceIDs, req1Step2.ID)
		assert.Contains(tb, instanceIDs, req2Step2.ID)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.discovery1", "test.operation.discovery2"),
	)
}

// Test 14: Workflow Data Management Tests
func TestWorkflowDataManagement(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "dataManagementTestWorkflow"
		operationName := "test.operation.datamanagement"

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow with initial data
		initialData := map[string]interface{}{"key1": "value1", "key2": "value2"}
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(initialData))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Verify initial data
		var metadata WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)

		var storedData map[string]interface{}
		err = json.Unmarshal(metadata.Data, &storedData)
		require.NoError(tb, err)
		assert.Equal(tb, initialData, storedData)

		// Update workflow data
		newData := map[string]interface{}{"key2": "updated_value2", "key3": "value3"}
		err = workflowService.UpdateWorkflowData(context.Background(), req.ID, newData)
		assert.NoError(tb, err)

		// Verify updated data
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		assert.NoError(tb, err)

		var updatedMetadata WorkflowMetadata
		err = json.Unmarshal(updatedReq.Metadata, &updatedMetadata)
		require.NoError(tb, err)

		var updatedStoredData map[string]interface{}
		err = json.Unmarshal(updatedMetadata.Data, &updatedStoredData)
		require.NoError(tb, err)

		// Should have merged data
		expectedData := map[string]interface{}{"key1": "value1", "key2": "updated_value2", "key3": "value3"}
		assert.Equal(tb, expectedData, updatedStoredData)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.datamanagement"),
	)
}

// Test 15: Progress Calculation Tests
func TestProgressCalculation(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "progressTestWorkflow"
		operationName1 := "test.operation.progress1"
		operationName2 := "test.operation.progress2"
		operationName3 := "test.operation.progress3"

		steps := []core.OperationStep{
			{
				Operation:  operationName1,
				Foreground: true,
			},
			{
				Operation:  operationName2,
				Foreground: true,
			},
			{
				Operation:  operationName3,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req1, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req1)

		// Initial progress should be 0%
		status, err := workflowService.GetWorkflowStatus(context.Background(), req1.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, 0.0, status.Progress)

		// Complete first step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req1.ID)
		assert.NoError(tb, err)

		// Find second request
		var req2 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName2, models.RequestStatusProcessing).First(&req2).Error
		require.NoError(tb, err)

		// Progress should be around 50% (1 step completed + 50% of current step)
		status, err = workflowService.GetWorkflowStatus(context.Background(), req2.ID)
		assert.NoError(tb, err)
		assert.InDelta(tb, 50.0, status.Progress, 1.0)

		// Complete second step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req2.ID)
		assert.NoError(tb, err)

		// Find third request
		var req3 models.Request
		err = ctx.DB().Model(&models.Request{}).Where("operation = ? AND status = ?", operationName3, models.RequestStatusProcessing).First(&req3).Error
		require.NoError(tb, err)

		// Progress should be around 83% (2 steps completed + 50% of current step)
		status, err = workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		assert.NoError(tb, err)
		assert.InDelta(tb, 83.33, status.Progress, 1.0)

		// Complete third step
		err = workflowService.ExecuteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)
		err = workflowService.CompleteWorkflowStep(context.Background(), req3.ID)
		assert.NoError(tb, err)

		// Final progress should be 100%
		status, err = workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, 100.0, status.Progress)

		// Verify status after cleanup also shows 100% progress
		status, err = workflowService.GetWorkflowStatus(context.Background(), req3.ID)
		assert.NoError(tb, err)
		assert.Equal(tb, models.RequestStatusCompleted, status.Status)
		assert.Equal(tb, 100.0, status.Progress)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.progress1", "test.operation.progress2", "test.operation.progress3"),
	)
}

// Test 16: Workflow Start with Request Data Tests
func TestWorkflowStartWithRequestData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "requestDataTestWorkflow"
		operationName := "test.operation.requestdata"

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Create test data for hash
		testData := []byte("test data for multihash")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		require.NoError(tb, err)

		// Create an initial request with all fields
		initialRequest := &models.Request{
			Protocol:  "testProtocol",
			UserID:    lo.ToPtr(uint(123)),
			SourceIP:  "127.0.0.1",
			Hash:      mh,
			CIDType:   1,
			Operation: operationName,
			Status:    models.RequestStatusPending,
			Metadata:  datatypes.JSON([]byte(`{"test":"metadata"}`)),
		}

		// Start workflow with initial request as data
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowRequestData(initialRequest))
		assert.NoError(tb, err)
		assert.NotNil(tb, req)
		assert.Equal(tb, operationName, req.Operation)
		assert.Equal(tb, "testProtocol", req.Protocol)
		assert.Equal(tb, lo.ToPtr(uint(123)), req.UserID)
		assert.Equal(tb, "127.0.0.1", req.SourceIP)
		assert.Equal(tb, mh, req.Hash)
		assert.Equal(tb, uint64(1), req.CIDType)

		// Verify workflow metadata was set correctly
		var metadata WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		assert.Equal(tb, workflowName, metadata.WorkflowName)
		assert.Equal(tb, "step-test.operation.requestdata-0", metadata.CurrentStepID)
		assert.Equal(tb, 1, metadata.TotalSteps)
		assert.Equal(tb, uint(0), metadata.PrevRequestID)
		assert.Equal(tb, uint(0), metadata.NextRequestID)

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.requestdata"),
	)
}

// Test 18: GetWorkflowMetadata Tests
func TestGetWorkflowMetadata(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "metadataTestWorkflow"
		operationName := "test.operation.metadata"
		initialData := map[string]interface{}{"key1": "value1", "nested": map[string]interface{}{"key2": "value2"}}

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(initialData))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Get workflow metadata
		k, err := workflowService.GetWorkflowMetadata(context.Background(), req.ID)
		assert.NoError(tb, err)
		assert.NotNil(tb, k)

		// Verify data was correctly stored and retrieved
		assert.Equal(tb, "value1", k.String("key1"))
		assert.Equal(tb, "value2", k.String("nested.key2"))

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.metadata"),
	)
}

// Test 19: UpdateWorkflowDataStruct Tests
func TestUpdateWorkflowDataStruct(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		require.NotNil(tb, requestService)

		workflowName := "structDataTestWorkflow"
		operationName := "test.operation.structdata"

		steps := []core.OperationStep{
			{
				Operation:  operationName,
				Foreground: true,
			},
		}

		// Register workflow
		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		// Start workflow
		req, err := workflowService.StartWorkflow(context.Background(), workflowName, core.WithWorkflowData(map[string]interface{}{"initial": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		// Define a struct to update workflow data with
		type TestData struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		testStruct := TestData{
			Name:  "testName",
			Value: 42,
		}

		// Update workflow data with struct
		err = workflowService.UpdateWorkflowDataStruct(context.Background(), req.ID, testStruct, "json")
		assert.NoError(tb, err)

		// Verify the struct data was correctly stored
		updatedReq, err := requestService.GetRequest(context.Background(), req.ID)
		assert.NoError(tb, err)

		var metadata WorkflowMetadata
		err = json.Unmarshal(updatedReq.Metadata, &metadata)
		require.NoError(tb, err)

		var storedData map[string]interface{}
		err = json.Unmarshal(metadata.Data, &storedData)
		require.NoError(tb, err)

		assert.Equal(tb, "testName", storedData["name"])
		assert.Equal(tb, float64(42), storedData["value"]) // JSON numbers are float64

	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, NewWorkflowCoordinator),
		withTestProtocol("test.operation.structdata"),
	)
}

func withTestProtocol(operationNames ...string) coreTesting.TestContextBuilderOption {
	return coreTesting.CombineOptions(
		coreTesting.WithMockProtocol("test", func(protocol *coreTesting.MockProtocol) {
			// Register test operations for each provided name
			for _, name := range operationNames {
				op := coreTesting.NewMockOperation(protocol.TB()).
					WithType(func() string { return name }).
					WithHandler(func() core.OperationHandler {
						return coreTesting.NewMockOperationHandler(protocol.TB())
					})
				protocol.WithOperation(op)
			}
		}),
		coreTesting.WithProtocolConfig("test", coreTesting.NewMockConfigEntry()),
	)
}
