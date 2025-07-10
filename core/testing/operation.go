package testing

import (
	"context"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

// MockOperation implements core.Operation for testing with default expectations
type MockOperation struct {
	*mocks.MockOperation
	typeFunc       func() string
	globalTypeFunc func() core.OperationType
	handlerFunc    func() core.OperationHandler
}

// MockOperationHandler implements core.OperationHandler for testing with default expectations
type MockOperationHandler struct {
	*mocks.MockOperationHandler
	validateRequestFunc func(ctx context.Context, req *models.Request) error
	executeFunc         func(ctx context.Context, req *models.Request) error
	getStatusFunc       func(ctx context.Context, req *models.Request) (*core.RequestStatus, error)
	cleanupFunc         func(ctx context.Context, req *models.Request) error
}

// Type implements core.Operation
func (m *MockOperation) Type() string {
	if m.typeFunc != nil {
		return m.typeFunc()
	}
	return m.MockOperation.Type()
}

// GlobalType implements core.Operation
func (m *MockOperation) GlobalType() core.OperationType {
	if m.globalTypeFunc != nil {
		return m.globalTypeFunc()
	}
	return m.MockOperation.GlobalType()
}

// Handler implements core.Operation
func (m *MockOperation) Handler() core.OperationHandler {
	if m.handlerFunc != nil {
		return m.handlerFunc()
	}
	return m.MockOperation.Handler()
}

// ValidateRequest implements core.OperationHandler
func (m *MockOperationHandler) ValidateRequest(ctx context.Context, req *models.Request) error {
	if m.validateRequestFunc != nil {
		return m.validateRequestFunc(ctx, req)
	}
	return m.MockOperationHandler.ValidateRequest(ctx, req)
}

// Execute implements core.OperationHandler
func (m *MockOperationHandler) Execute(ctx context.Context, req *models.Request) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
	}
	return m.MockOperationHandler.Execute(ctx, req)
}

// GetStatus implements core.OperationHandler
func (m *MockOperationHandler) GetStatus(ctx context.Context, req *models.Request) (*core.RequestStatus, error) {
	if m.getStatusFunc != nil {
		return m.getStatusFunc(ctx, req)
	}
	return m.MockOperationHandler.GetStatus(ctx, req)
}

// Cleanup implements core.OperationHandler
func (m *MockOperationHandler) Cleanup(ctx context.Context, req *models.Request) error {
	if m.cleanupFunc != nil {
		return m.cleanupFunc(ctx, req)
	}
	return m.MockOperationHandler.Cleanup(ctx, req)
}

// NewMockOperation creates a new mock Operation with default expectations
func NewMockOperation(t TB) *MockOperation {
	mockOp := mocks.NewMockOperation(t)
	op := &MockOperation{
		MockOperation: mockOp,
	}

	// Set up default expectations (optional, but good practice)
	mockOp.On("Type").Return("").Maybe()
	mockOp.On("GlobalType").Return(core.OperationType("")).Maybe()
	mockOp.On("Handler").Return(NewMockOperationHandler(t)).Maybe()

	return op
}

// NewMockOperationHandler creates a new mock OperationHandler with default expectations
func NewMockOperationHandler(t TB) *MockOperationHandler {
	mockHandler := mocks.NewMockOperationHandler(t)
	handler := &MockOperationHandler{
		MockOperationHandler: mockHandler,
	}

	// Set up default expectations (optional, but good practice)
	mockHandler.On("ValidateRequest", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockHandler.On("Execute", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockHandler.On("GetStatus", mock.Anything, mock.Anything).Return(core.RequestStatus{}, nil).Maybe()
	mockHandler.On("Cleanup", mock.Anything, mock.Anything).Return(nil).Maybe()

	return handler
}

// WithType sets a custom Type function for the MockOperation
func (m *MockOperation) WithType(f func() string) *MockOperation {
	m.typeFunc = f
	return m
}

// WithGlobalType sets a custom GlobalType function for the MockOperation
func (m *MockOperation) WithGlobalType(f func() core.OperationType) *MockOperation {
	m.globalTypeFunc = f
	return m
}

// WithHandler sets a custom Handler function for the MockOperation
func (m *MockOperation) WithHandler(f func() core.OperationHandler) *MockOperation {
	m.handlerFunc = f
	return m
}

// WithValidateRequest sets a custom ValidateRequest function for the MockOperationHandler
func (m *MockOperationHandler) WithValidateRequest(f func(ctx context.Context, req *models.Request) error) *MockOperationHandler {
	m.validateRequestFunc = f
	return m
}

// WithExecute sets a custom Execute function for the MockOperationHandler
func (m *MockOperationHandler) WithExecute(f func(ctx context.Context, req *models.Request) error) *MockOperationHandler {
	m.executeFunc = f
	return m
}

// WithGetStatus sets a custom GetStatus function for the MockOperationHandler
func (m *MockOperationHandler) WithGetStatus(f func(ctx context.Context, req *models.Request) (*core.RequestStatus, error)) *MockOperationHandler {
	m.getStatusFunc = f
	return m
}

// WithCleanup sets a custom Cleanup function for the MockOperationHandler
func (m *MockOperationHandler) WithCleanup(f func(ctx context.Context, req *models.Request) error) *MockOperationHandler {
	m.cleanupFunc = f
	return m
}

// Ensure MockOperation implements core.Operation
var _ core.Operation = (*MockOperation)(nil)

// Ensure MockOperationHandler implements core.OperationHandler
var _ core.OperationHandler = (*MockOperationHandler)(nil)
