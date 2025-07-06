package testing

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"testing"
)

// WorkflowTest encapsulates common setup and assertion logic for workflow tests.
type WorkflowTest struct {
	TB          testing.TB
	Ctx         TestContext
	workflowSvc core.WorkflowService
}

// NewWorkflowTest creates a new WorkflowTest instance.
func NewWorkflowTest(ctx TestContext) *WorkflowTest {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
	return &WorkflowTest{
		TB:          ctx.T(),
		Ctx:         ctx,
		workflowSvc: workflowSvc,
	}
}

// RegisterWorkflow registers a workflow with the given steps.
func (wt *WorkflowTest) RegisterWorkflow(workflowName string, steps []core.OperationStep, autoTriggerFirstStep bool) {
	err := wt.workflowSvc.RegisterWorkflow(workflowName, steps, autoTriggerFirstStep)
	require.NoError(wt.TB, err)
}

// StartWorkflow starts a workflow with the given name and options.
func (wt *WorkflowTest) StartWorkflow(workflowName string, opts ...core.WorkflowOption) *models.Request {
	req, err := wt.workflowSvc.StartWorkflow(wt.Ctx, workflowName, opts...)
	require.NoError(wt.TB, err)
	return req
}

// AssertRequestStatus asserts that the request has the given status.
func (wt *WorkflowTest) AssertRequestStatus(requestID uint, expectedStatus models.RequestStatusType) {
	requestSvc := core.GetService[core.RequestService](wt.Ctx, core.REQUEST_SERVICE)
	req, err := requestSvc.GetRequest(wt.Ctx, requestID)
	require.NoError(wt.TB, err)
	assert.Equal(wt.TB, expectedStatus, req.Status)
}

// AssertMetadataValue asserts that the metadata contains the given key-value pair.
func (wt *WorkflowTest) AssertMetadataValue(requestID uint, key string, expectedValue any) {
	metadata, err := wt.workflowSvc.GetWorkflowMetadata(wt.Ctx, requestID)
	require.NoError(wt.TB, err, "failed to get workflow metadata")
	require.NotNil(wt.TB, metadata, "metadata should not be nil")

	actualValue := metadata.Get(key)
	assert.Equal(wt.TB, expectedValue, actualValue, "metadata value mismatch for key %q", key)
}

// NewOperationWorkflow creates and registers a simple workflow with a single operation.
func (wt *WorkflowTest) NewOperationWorkflow(operationName string) string {
	workflowName := fmt.Sprintf("test-workflow-%s", operationName) // Unique name
	steps := []core.OperationStep{{Operation: operationName, FailureBehavior: core.FailWorkflow, Foreground: true}}
	wt.RegisterWorkflow(workflowName, steps, false) // Don't auto-trigger
	return workflowName
}

// StartOperationWorkflow starts a workflow with the given operation and options.
func (wt *WorkflowTest) StartOperationWorkflow(operationName string, opts ...core.WorkflowOption) *models.Request {
	workflowName := wt.NewOperationWorkflow(operationName)
	return wt.StartWorkflow(workflowName, opts...)
}

// AssertOperationSuccess asserts that the operation completed successfully.
func (wt *WorkflowTest) AssertOperationSuccess(req *models.Request) {
	wt.AssertRequestStatus(req.ID, models.RequestStatusCompleted)
}

// AssertOperationFailed asserts that the operation failed.
func (wt *WorkflowTest) AssertOperationFailed(req *models.Request) {
	wt.AssertRequestStatus(req.ID, models.RequestStatusFailed)
}

// AssertOperationStatusMessageContains asserts that the request status message contains the given string.
func (wt *WorkflowTest) AssertOperationStatusMessageContains(req *models.Request, expectedMessage string) {
	requestSvc := core.GetService[core.RequestService](wt.Ctx, core.REQUEST_SERVICE)
	updatedReq, err := requestSvc.GetRequest(wt.Ctx, req.ID)
	require.NoError(wt.TB, err)
	assert.Contains(wt.TB, updatedReq.StatusMessage, expectedMessage)
}

// AssertOperationStatusProgress asserts that the request status progress is equal to the given value.
func (wt *WorkflowTest) AssertOperationStatusProgress(req *models.Request, expectedProgress int) {
	requestSvc := core.GetService[core.RequestService](wt.Ctx, core.REQUEST_SERVICE)
	updatedReq, err := requestSvc.GetRequestStatus(wt.Ctx, req.ID)
	require.NoError(wt.TB, err)
	assert.Equal(wt.TB, expectedProgress, updatedReq.ProgressPercent)
}

// ExecuteWorkflowStep executes the workflow step for the given request.
func (wt *WorkflowTest) ExecuteWorkflowStep(req *models.Request) {
	err := wt.workflowSvc.ExecuteWorkflowStep(context.Background(), req.ID)
	require.NoError(wt.TB, err)
}
