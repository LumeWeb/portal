package testing

import (
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
