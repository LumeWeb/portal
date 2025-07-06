package core

import (
	"context"
	"fmt"
	"github.com/knadh/koanf/v2"
	"go.lumeweb.com/portal/db/models"
	"reflect"
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
	// WorkflowData retrieves workflow metadata for a request as a koanf instance
	WorkflowData(requestID uint) (*koanf.Koanf, error)
	// StructuredWorkflowData retrieves workflow metadata and unmarshals it into the provided struct
	StructuredWorkflowData(requestID uint, out any) error
	// UpdateWorkflowData updates workflow metadata with the provided data map
	UpdateWorkflowData(requestID uint, data map[string]any) error
	// UpdateWorkflowDataStruct updates workflow metadata with the provided struct
	UpdateWorkflowDataStruct(requestID uint, data any) error
	// Protocol retrieves a protocol instance by ID
	Protocol() Protocol
	// StorageHash retrieves the storage hash from the current request
	StorageHash(req *models.Request) StorageHash
	// Context returns the operation context
	Context() Context
	// Logger returns the operation logger
	Logger() *Logger
	// StartWorkflow starts a new workflow with the given name and options
	StartWorkflow(name string, opts ...WorkflowOption) (*models.Request, error)
}

// OperationHelperDefault is the default implementation of OperationHelper
type OperationHelperDefault struct {
	ctx       Context
	proto     string
	unmarshalTag string // Tag to use for unmarshaling (default: "json")
	workflow  WorkflowService // Cached workflow service instance
}

// DefaultUnmarshalTag is the default tag used for unmarshaling
const DefaultUnmarshalTag = "json"

// NewProtocolOperationHelper creates a new OperationHelper instance with protocol context
func NewOperationHelper(ctx Context) OperationHelper {
	return &OperationHelperDefault{
		ctx:          ctx,
		unmarshalTag: DefaultUnmarshalTag,
		workflow:     GetService[WorkflowService](ctx, WORKFLOW_SERVICE),
	}
}

// NewOperationHelper creates a new OperationHelper instance
func NewProtocolOperationHelper(ctx Context, proto string) OperationHelper {
	return &OperationHelperDefault{
		ctx:          ctx,
		proto:        proto,
		unmarshalTag: DefaultUnmarshalTag,
		workflow:     GetService[WorkflowService](ctx, WORKFLOW_SERVICE),
	}
}

// WithUnmarshalTag sets the tag to use for unmarshaling
func (h *OperationHelperDefault) WithUnmarshalTag(tag string) *OperationHelperDefault {
	h.unmarshalTag = tag
	return h
}

// GetWorkflowData retrieves workflow metadata for a request
func (h *OperationHelperDefault) WorkflowData(requestID uint) (*koanf.Koanf, error) {
	// Get workflow service
	workflow := GetService[WorkflowService](h.ctx, WORKFLOW_SERVICE)

	// Get the workflow metadata
	return workflow.GetWorkflowMetadata(h.ctx, requestID)
}

// StructuredWorkflowData retrieves workflow metadata and unmarshals it into the provided struct
// out must be a non-nil pointer to a struct or an interface
func (h *OperationHelperDefault) StructuredWorkflowData(requestID uint, out any) error {
	if out == nil {
		return fmt.Errorf("out parameter cannot be nil")
	}

	val := reflect.ValueOf(out)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return fmt.Errorf("out parameter must be a non-nil pointer")
	}

	k, err := h.WorkflowData(requestID)
	if err != nil {
		return err
	}

	return k.UnmarshalWithConf("", out, koanf.UnmarshalConf{Tag: h.unmarshalTag})
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

// UpdateWorkflowData updates workflow metadata with the provided data map
func (h *OperationHelperDefault) UpdateWorkflowData(requestID uint, data map[string]any) error {
	return h.workflow.UpdateWorkflowData(h.ctx, requestID, data)
}

// UpdateWorkflowDataStruct updates workflow metadata with the provided struct
func (h *OperationHelperDefault) UpdateWorkflowDataStruct(requestID uint, data any) error {
	return h.workflow.UpdateWorkflowDataStruct(h.ctx, requestID, data, h.unmarshalTag)
}

// StartWorkflow starts a new workflow with the given name and options
func (h *OperationHelperDefault) StartWorkflow(name string, opts ...WorkflowOption) (*models.Request, error) {
	return h.workflow.StartWorkflow(h.ctx, name, opts...)
}
