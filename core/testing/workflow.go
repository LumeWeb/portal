package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// WorkflowTest encapsulates common setup and assertion logic for workflow tests.
type WorkflowTest struct {
	TB          testing.TB
	Ctx         TestContext
	workflowSvc core.WorkflowService
	requestSvc  core.RequestService
}

// NewWorkflowTest creates a new WorkflowTest instance.
func NewWorkflowTest(ctx TestContext) *WorkflowTest {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
	requestSvc := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
	return &WorkflowTest{
		TB:          ctx.T(),
		Ctx:         ctx,
		workflowSvc: workflowSvc,
		requestSvc:  requestSvc,
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
	req, err := wt.requestSvc.GetRequestStatus(wt.Ctx, requestID, true)
	require.NoError(wt.TB, err)
	assert.EqualValues(wt.TB, expectedStatus, req.State)
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
	reqStatus, err := wt.requestSvc.GetRequestStatus(wt.Ctx, req.ID, true)
	require.NoError(wt.TB, err)
	assert.Contains(wt.TB, reqStatus.Message, expectedMessage)
}

// AssertOperationStatusProgress asserts that the request status progress is equal to the given value.
func (wt *WorkflowTest) AssertOperationStatusProgress(req *models.Request, expectedProgress float64) {
	updatedReq, err := wt.requestSvc.GetRequestStatus(wt.Ctx, req.ID, true)
	require.NoError(wt.TB, err)
	assert.Equal(wt.TB, expectedProgress, updatedReq.ProgressPercent)
}

// ExecuteWorkflowStep executes the workflow step for the given request.
func (wt *WorkflowTest) ExecuteWorkflowStep(req *models.Request) {
	err := wt.workflowSvc.ExecuteWorkflowStep(context.Background(), req.ID)
	require.NoError(wt.TB, err)
}

// DisableWorkflow disables the workflow with the given name.
func (wt *WorkflowTest) DisableWorkflow(workflowName string) {
	err := wt.workflowSvc.DisableWorkflow(workflowName)
	require.NoError(wt.TB, err)
}

// EnableWorkflow enables the workflow with the given name.
func (wt *WorkflowTest) EnableWorkflow(workflowName string) {
	err := wt.workflowSvc.EnableWorkflow(workflowName)
	require.NoError(wt.TB, err)
}

// ConvertRequestToWorkflow converts an existing request to a workflow with the given name and options.
func (wt *WorkflowTest) ConvertRequestToWorkflow(requestID uint, workflowName string, startStep int, opts ...core.WorkflowOption) error {
	return wt.workflowSvc.ConvertRequestToWorkflow(wt.Ctx, requestID, workflowName, startStep, opts...)
}

// MustConvertRequestToWorkflow converts an existing request to a workflow and fails the test if it errors.
func (wt *WorkflowTest) MustConvertRequestToWorkflow(requestID uint, workflowName string, startStep int, opts ...core.WorkflowOption) {
	err := wt.ConvertRequestToWorkflow(requestID, workflowName, startStep, opts...)
	require.NoError(wt.TB, err)
}

// GetRequest retrieves a request by ID and fails the test if it errors.
func (wt *WorkflowTest) GetRequest(requestID uint) *models.Request {
	req, err := wt.requestSvc.GetRequest(wt.Ctx, requestID)
	require.NoError(wt.TB, err)
	return req
}

// FindWorkflowInstances finds workflow instances matching the criteria and fails the test if it errors
func (wt *WorkflowTest) FindWorkflowInstances(workflowName string, filter core.RequestFilter) []*core.WorkflowInstance {
	instances, err := wt.workflowSvc.FindWorkflowInstances(wt.Ctx, workflowName, filter)
	require.NoError(wt.TB, err)
	return instances
}

// FindFirstWorkflowInstance finds the first workflow instance matching the criteria and fails the test if it errors or none found
func (wt *WorkflowTest) FindFirstWorkflowInstance(workflowName string, filter core.RequestFilter) *core.WorkflowInstance {
	instances := wt.FindWorkflowInstances(workflowName, filter)
	require.NotEmpty(wt.TB, instances, "no workflow instances found matching criteria")
	return instances[0]
}

// WaitForWorkflowInstance waits for a workflow instance matching the criteria to appear, with timeout
func (wt *WorkflowTest) WaitForWorkflowInstance(workflowName string, filter core.RequestFilter, timeout time.Duration) *core.WorkflowInstance {
	var instance *core.WorkflowInstance
	require.Eventually(wt.TB, func() bool {
		instances := wt.FindWorkflowInstances(workflowName, filter)
		if len(instances) > 0 {
			instance = instances[0]
			return true
		}
		return false
	}, timeout, 100*time.Millisecond, "timed out waiting for workflow instance")
	return instance
}
