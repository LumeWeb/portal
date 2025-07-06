package core

import (
	"context"
	"fmt"
	"github.com/knadh/koanf/v2"
	"go.lumeweb.com/portal/db/models"
)

// Default implementation
type operation struct {
	opType     string
	globalType OperationType
	handler    OperationHandler
}

func (o *operation) Type() string {
	return o.opType
}

func (o *operation) GlobalType() OperationType {
	return o.globalType
}

func (o *operation) Handler() OperationHandler {
	return o.handler
}

type OperationType string

var (
	// Network operations
	OpTypeRetrieve OperationType = "retrieve" // Fetch from network
	OpTypePublish  OperationType = "publish"  // Publish to network

	// Storage operations
	OpTypeStore   OperationType = "store"   // Store locally
	OpTypeUnstore OperationType = "unstore" // Remove local storage

	// Data operations
	OpTypeUpload OperationType = "upload" // Upload new content
	OpTypeScan   OperationType = "scan"   // Scan/validate content
)

type Operation interface {
	Type() string              // Specific operation name (e.g., "ipfs.pin" or "ipfs.pin.car")
	GlobalType() OperationType // Optional global type mapping (e.g., OpTypePin) or empty for custom ops
	Handler() OperationHandler // The handler implementation
}

type OperationHandler interface {
	// Validates operation request before creating it
	ValidateRequest(ctx context.Context, req *models.Request) error

	// Executes the actual operation
	Execute(ctx context.Context, req *models.Request) error

	// Gets current status and progress of an ongoing operation
	GetStatus(ctx context.Context, req *models.Request) (RequestStatus, error)

	// Optional cleanup after operation completes or fails
	Cleanup(ctx context.Context, req *models.Request) error
}

func NewOperation(opType string, globalType OperationType, handler OperationHandler) Operation {
	return &operation{
		opType:     opType,
		globalType: globalType,
		handler:    handler,
	}
}

func NewStoreOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		fmt.Sprintf("%s.store", protocol),
		OpTypeStore,
		handler,
	)
}

func NewRetrieveOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		fmt.Sprintf("%s.retrieve", protocol),
		OpTypeRetrieve,
		handler,
	)
}

func NewPublishOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		fmt.Sprintf("%s.publish", protocol),
		OpTypePublish,
		handler,
	)
}

func NewUploadOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		fmt.Sprintf("%s.upload", protocol),
		OpTypeUpload,
		handler,
	)
}

func NewScanOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		fmt.Sprintf("%s.scan", protocol),
		OpTypeScan,
		handler,
	)
}

// OperationHelper provides helper methods for working with operations
type OperationHelper interface {
	// WorkflowData retrieves workflow metadata for a request
	WorkflowData(requestID uint) (*koanf.Koanf, error)
	// Protocol retrieves a protocol instance by ID
	Protocol() Protocol
	// StorageHash retrieves the storage hash from the current request
	StorageHash(req *models.Request) StorageHash
	// Context returns the operation context
	Context() Context
	// Logger returns the operation logger
	Logger() *Logger
}

// OperationHelperDefault is the default implementation of OperationHelper
type OperationHelperDefault struct {
	ctx   Context
	proto string
}

// NewProtocolOperationHelper creates a new OperationHelper instance with protocol context
func NewOperationHelper(ctx Context) OperationHelper {
	return &OperationHelperDefault{
		ctx: ctx,
	}
}

// NewOperationHelper creates a new OperationHelper instance
func NewProtocolOperationHelper(ctx Context, proto string) OperationHelper {
	return &OperationHelperDefault{
		ctx:   ctx,
		proto: proto,
	}
}

// GetWorkflowData retrieves workflow metadata for a request
func (h *OperationHelperDefault) WorkflowData(requestID uint) (*koanf.Koanf, error) {
	// Get workflow service
	workflow := GetService[WorkflowService](h.ctx, WORKFLOW_SERVICE)

	// Get the workflow metadata
	return workflow.GetWorkflowMetadata(h.ctx, requestID)
}

// Protocol retrieves a protocol instance by ID
func (h *OperationHelperDefault) Protocol() Protocol {
	if h.proto == "" {
		h.ctx.Logger().Fatal("protocol not set in OperationHelper")
	}

	// Get the protocol instance
	return GetProtocol(h.proto)
}

// StorageHash retrieves the storage hash from the provided request
func (h *OperationHelperDefault) StorageHash(req *models.Request) StorageHash {
	if req == nil {
		return nil
	}

	// Create storage hash from request data
	if req.Hash == nil {
		return nil
	}

	return NewStorageHashFromMultihash(req.Hash, req.CIDType, nil)
}

// Context returns the operation context
func (h *OperationHelperDefault) Context() Context {
	return h.ctx
}

// Logger returns the operation logger
func (h *OperationHelperDefault) Logger() *Logger {
	return h.ctx.Logger()
}
