package testing

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

// MockWorkflowService provides high-level helper functions for workflow testing.
// It simplifies common workflow mocking patterns and provides convenient methods
// for starting workflows with automatic mock setup.
type MockWorkflowService struct {
	*mocks.MockWorkflowService
	ctx TestContext
}

// NewMockWorkflowService creates a new MockWorkflowService.
func NewMockWorkflowService(ctx TestContext) *MockWorkflowService {
	return &MockWorkflowService{
		MockWorkflowService: mocks.NewMockWorkflowService(ctx.T()),
		ctx:                 ctx,
	}
}

// ExpectStartWorkflow sets up a mock expectation for StartWorkflow with flexible argument matching.
// This handles the variadic WorkflowOption arguments that cause test failures.
func (m *MockWorkflowService) ExpectStartWorkflow(workflowName string, request *models.Request, err error) *mocks.MockWorkflowService_StartWorkflow_Call {
	// Use mock.Anything for context and variadic options to match any call signature
	return m.EXPECT().StartWorkflow(mock.Anything, workflowName, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(request, err)
}

// ExpectStartWorkflowWithMinArgs sets up a mock expectation for StartWorkflow with minimum arguments.
// Use this when you know exactly how many workflow options will be passed.
func (m *MockWorkflowService) ExpectStartWorkflowWithMinArgs(workflowName string, request *models.Request, err error, optionCount int) *mocks.MockWorkflowService_StartWorkflow_Call {
	// Build the argument list with the exact number of options
	args := []interface{}{mock.Anything, workflowName}
	for i := 0; i < optionCount; i++ {
		args = append(args, mock.Anything)
	}
	return m.EXPECT().StartWorkflow(args...).Return(request, err)
}

// SetupSimpleWorkflow creates a simple workflow mock setup for common API testing scenarios.
// It registers a workflow and sets up StartWorkflow expectations.
func (m *MockWorkflowService) SetupSimpleWorkflow(workflowName string, expectedRequest *models.Request) {
	// Mock workflow registration
	m.EXPECT().RegisterWorkflow(workflowName, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Mock StartWorkflow with flexible argument matching
	m.ExpectStartWorkflow(workflowName, expectedRequest, nil)
}

// SetupOperationWorkflow creates a workflow mock for a single operation workflow.
// This is the most common pattern used in API tests.
func (m *MockWorkflowService) SetupOperationWorkflow(operationName string, expectedRequest *models.Request) {
	workflowName := fmt.Sprintf("test-workflow-%s", operationName)
	m.SetupSimpleWorkflow(workflowName, expectedRequest)
}

// SetupFailingWorkflow creates a workflow mock that returns an error.
// Useful for testing error handling paths.
func (m *MockWorkflowService) SetupFailingWorkflow(workflowName string, expectedError error) {
	// Mock workflow registration
	m.EXPECT().RegisterWorkflow(workflowName, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Mock StartWorkflow to return error
	m.ExpectStartWorkflow(workflowName, nil, expectedError)
}

// SetupOperationWorkflowFailing creates a failing workflow mock for a single operation.
func (m *MockWorkflowService) SetupOperationWorkflowFailing(operationName string, expectedError error) {
	workflowName := fmt.Sprintf("test-workflow-%s", operationName)
	m.SetupFailingWorkflow(workflowName, expectedError)
}

// CreateTestRequest creates a test request object for use in workflow mocks.
func (m *MockWorkflowService) CreateTestRequest(operationName string, userID uint) *models.Request {
	return &models.Request{
		Model:          models.Model{ID: 1},
		Operation:      operationName,
		State:          models.RequestStatusPending,
		ProgressPercent: 0.0,
		UserID:         userID,
	}
}

// CreateCompletedTestRequest creates a completed test request object.
func (m *MockWorkflowService) CreateCompletedTestRequest(operationName string, userID uint) *models.Request {
	return &models.Request{
		Model:          models.Model{ID: 1},
		Operation:      operationName,
		State:          models.RequestStatusCompleted,
		ProgressPercent: 100.0,
		UserID:         userID,
	}
}

// CreateFailedTestRequest creates a failed test request object.
func (m *MockWorkflowService) CreateFailedTestRequest(operationName string, userID uint, errorMessage string) *models.Request {
	return &models.Request{
		Model:          models.Model{ID: 1},
		Operation:      operationName,
		State:          models.RequestStatusFailed,
		ProgressPercent: 0.0,
		UserID:         userID,
		Error:          errorMessage,
	}
}

// ExpectGetWorkflowStatus sets up a mock expectation for GetWorkflowStatus.
func (m *MockWorkflowService) ExpectGetWorkflowStatus(requestID uint, status *core.WorkflowStatus, err error) *mocks.MockWorkflowService_GetWorkflowStatus_Call {
	return m.EXPECT().GetWorkflowStatus(mock.Anything, requestID).Return(status, err)
}

// CreateTestWorkflowStatus creates a test workflow status object.
func (m *MockWorkflowService) CreateTestWorkflowStatus(currentStep string, state models.RequestStatusType) *core.WorkflowStatus {
	return &core.WorkflowStatus{
		CurrentStep: currentStep,
		State:       state,
		Completed:   state == models.RequestStatusCompleted,
		Failed:      state == models.RequestStatusFailed,
	}
}

// SetupCompleteWorkflowScenario sets up a complete workflow scenario with status checks.
// This is useful for testing workflows that need to be queried for status after starting.
func (m *MockWorkflowService) SetupCompleteWorkflowScenario(workflowName string, request *models.Request) {
	// Setup workflow start
	m.SetupSimpleWorkflow(workflowName, request)

	// Setup status check
	status := m.CreateTestWorkflowStatus("", models.RequestStatusCompleted)
	m.ExpectGetWorkflowStatus(request.ID, status, nil)
}

// SetupFailingWorkflowScenario sets up a complete failing workflow scenario.
func (m *MockWorkflowService) SetupFailingWorkflowScenario(workflowName string, request *models.Request, errorMessage string) {
	// Setup workflow start
	m.SetupSimpleWorkflow(workflowName, request)

	// Setup status check
	status := m.CreateTestWorkflowStatus("", models.RequestStatusFailed)
	m.ExpectGetWorkflowStatus(request.ID, status, nil)
}

// ExpectUpdateWorkflowData sets up a mock expectation for UpdateWorkflowData.
func (m *MockWorkflowService) ExpectUpdateWorkflowData(requestID uint, err error) *mocks.MockWorkflowService_UpdateWorkflowData_Call {
	return m.EXPECT().UpdateWorkflowData(mock.Anything, requestID, mock.Anything).Return(err)
}

// SetupWorkflowWithDataUpdate sets up a workflow that also expects data updates.
func (m *MockWorkflowService) SetupWorkflowWithDataUpdate(workflowName string, request *models.Request) {
	m.SetupSimpleWorkflow(workflowName, request)
	m.ExpectUpdateWorkflowData(request.ID, nil).Maybe()
}

// Helper method to get the test context
func (m *MockWorkflowService) GetContext() TestContext {
	return m.ctx
}

// Convenience method to get the underlying mock for advanced usage
func (m *MockWorkflowService) GetMock() *mocks.MockWorkflowService {
	return m.MockWorkflowService
}