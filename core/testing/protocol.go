package testing

import (
	"context"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// MockProtocol implements core.Protocol for testing
type MockProtocol struct {
	NameValue      string
	ConfigValue    config.ProtocolConfig
	OperationsValue []core.Operation
}

func (p *MockProtocol) Name() string {
	return p.NameValue
}

func (p *MockProtocol) Config() config.ProtocolConfig {
	return p.ConfigValue
}

func (p *MockProtocol) Operations() []core.Operation {
	return p.OperationsValue
}

// NewMockProtocol creates a new mock protocol
func NewMockProtocol(name string) *MockProtocol {
	return &MockProtocol{
		NameValue:      name,
		OperationsValue: []core.Operation{},
	}
}

// WithOperation adds an operation to the mock protocol
func (p *MockProtocol) WithOperation(op core.Operation) *MockProtocol {
	p.OperationsValue = append(p.OperationsValue, op)
	return p
}

// WithConfig sets the config for the mock protocol
func (p *MockProtocol) WithConfig(cfg config.ProtocolConfig) *MockProtocol {
	p.ConfigValue = cfg
	return p
}

// MockOperation implements core.Operation for testing
type MockOperation struct {
	TypeValue       string
	GlobalTypeValue core.OperationType
	HandlerValue    core.OperationHandler
}

func (o *MockOperation) Type() string {
	return o.TypeValue
}

func (o *MockOperation) GlobalType() core.OperationType {
	return o.GlobalTypeValue
}

func (o *MockOperation) Handler() core.OperationHandler {
	return o.HandlerValue
}

// NewMockOperation creates a new mock operation
func NewMockOperation(opType string, globalType core.OperationType) *MockOperation {
	return &MockOperation{
		TypeValue:       opType,
		GlobalTypeValue: globalType,
	}
}

// WithHandler sets the handler for the mock operation
func (o *MockOperation) WithHandler(handler core.OperationHandler) *MockOperation {
	o.HandlerValue = handler
	return o
}

// MockOperationHandler implements core.OperationHandler for testing
type MockOperationHandler struct {
	ValidateRequestFunc func(ctx context.Context, req *models.Request) error
	ExecuteFunc         func(ctx context.Context, req *models.Request) error
	GetStatusFunc       func(ctx context.Context, req *models.Request) (core.RequestStatus, error)
	CleanupFunc         func(ctx context.Context, req *models.Request) error
}

func (h *MockOperationHandler) ValidateRequest(ctx context.Context, req *models.Request) error {
	if h.ValidateRequestFunc != nil {
		return h.ValidateRequestFunc(ctx, req)
	}
	return nil
}

func (h *MockOperationHandler) Execute(ctx context.Context, req *models.Request) error {
	if h.ExecuteFunc != nil {
		return h.ExecuteFunc(ctx, req)
	}
	return nil
}

func (h *MockOperationHandler) GetStatus(ctx context.Context, req *models.Request) (core.RequestStatus, error) {
	if h.GetStatusFunc != nil {
		return h.GetStatusFunc(ctx, req)
	}
	return core.RequestStatus{
		State:   "completed",
		Message: "Mock operation completed",
	}, nil
}

func (h *MockOperationHandler) Cleanup(ctx context.Context, req *models.Request) error {
	if h.CleanupFunc != nil {
		return h.CleanupFunc(ctx, req)
	}
	return nil
}

// NewMockOperationHandler creates a new mock operation handler
func NewMockOperationHandler() *MockOperationHandler {
	return &MockOperationHandler{}
}
