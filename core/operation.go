package core

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
)

// Default implementation
type operation struct {
	opType     string
	globalType OperationType
	handler    OperationHandler
	name       string
}

// Ensure operation implements OperationDisplay
var _ OperationDisplay = (*operation)(nil)

func (o *operation) Type() string {
	return o.opType
}

func (o *operation) GlobalType() OperationType {
	return o.globalType
}

func (o *operation) Handler() OperationHandler {
	return o.handler
}

func (o *operation) Name() string {
	if o.name != "" {
		return o.name
	}

	// If this operation has a global type, use the display info from the map
	if o.globalType != "" {
		if info, exists := GetOperationTypeDisplayInfo(o.globalType); exists {
			return info.Name
		}
	}

	// Fallback to the operation type (e.g., "ipfs.pin")
	return o.opType
}

// DisplayName returns the human-readable display name for this operation
func (o *operation) DisplayName() string {
	return o.Name()
}

type OperationType string

// Operation category constants
const (
	OperationCategoryNetwork = "network"
	OperationCategoryStorage = "storage"
	OperationCategoryData    = "data"
)

// OperationTypeInfo contains human-readable display information for an operation type
type OperationTypeInfo struct {
	Name        string `json:"name"`        // Human-readable display name
	Description string `json:"description"` // Detailed description of the operation
	Category    string `json:"category"`    // Category of the operation (network, storage, data)
}

// OperationTypeDisplayNames maps operation types to their human-readable display information
var OperationTypeDisplayNames = map[OperationType]OperationTypeInfo{
	// Network operations
	OpTypeRetrieve: {Name: "Retrieve", Description: "Fetch content from network", Category: OperationCategoryNetwork},
	OpTypePublish:  {Name: "Publish", Description: "Publish content to network", Category: OperationCategoryNetwork},

	// Storage operations
	OpTypeStore:   {Name: "Store", Description: "Store content locally", Category: OperationCategoryStorage},
	OpTypeUnstore: {Name: "Unstore", Description: "Remove local storage", Category: OperationCategoryStorage},

	// Data operations
	OpTypeUpload: {Name: "Upload", Description: "Upload new content", Category: OperationCategoryData},
	OpTypeScan:   {Name: "Scan", Description: "Scan and validate content", Category: OperationCategoryData},
}

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

// Progress message provider fallback constants
const (
	progressProviderFallbackOperation = "operation"
)

type Operation interface {
	Type() string              // Specific operation name (e.g., "ipfs.pin" or "ipfs.pin.car")
	GlobalType() OperationType // Optional global type mapping (e.g., OpTypePin) or empty for custom ops
	Handler() OperationHandler // The handler implementation
	Name() string              // Custom name for the operation
}

// OperationDisplay provides human-readable display information for operations
type OperationDisplay interface {
	DisplayName() string // Human-readable display name for the operation
}

type OperationHandler interface {
	// Validates operation request before creating it
	ValidateRequest(ctx context.Context, req *models.Request) error

	// Executes the actual operation
	Execute(ctx context.Context, req *models.Request) error

	// Gets current status and progress of an ongoing operation
	GetStatus(ctx context.Context, req *models.Request) (*RequestStatus, error)

	// Optional cleanup after operation completes or fails
	Cleanup(ctx context.Context, req *models.Request) error
}

func operationName(protocol string, opType OperationType) string {
	return formatOperationName(protocol, strings.ToLower(string(opType)))
}

func prefixedOperationName(protocol string, prefix string, opType OperationType) string {
	return formatOperationName(protocol, prefix, strings.ToLower(string(opType)))
}

func tusOperationName(protocol string, opType OperationType) string {
	return prefixedOperationName(protocol, "tus", opType)
}

func postOperationName(protocol string, opType OperationType) string {
	return prefixedOperationName(protocol, "post", opType)
}

func StoreOperationName(protocol string) string {
	return operationName(protocol, OpTypeStore)
}

func RetrieveOperationName(protocol string) string {
	return operationName(protocol, OpTypeRetrieve)
}

func PublishOperationName(protocol string) string {
	return operationName(protocol, OpTypePublish)
}

func UploadOperationName(protocol string) string {
	return operationName(protocol, OpTypeUpload)
}

func TUSUploadOperationName(protocol string) string {
	return tusOperationName(protocol, OpTypeUpload)
}

func PostUploadOperationName(protocol string) string {
	return postOperationName(protocol, OpTypeUpload)
}

func ScanOperationName(protocol string) string {
	return operationName(protocol, OpTypeScan)
}

func UnstoreOperationName(protocol string) string {
	return operationName(protocol, OpTypeUnstore)
}

// OperationName creates an operation name by joining the protocol with additional parts
func OperationName(protocol string, parts ...string) string {
	allParts := append([]string{protocol}, parts...)
	return formatOperationName(allParts...)
}

// stringArgsToInterface converts a slice of strings to a slice of interfaces
// formatOperationName creates a format string with "%s.%s.%s..." pattern based on the number of arguments
func formatOperationName(parts ...string) string {
	// Filter out empty parts
	filteredParts := lo.Filter(parts, func(part string, _ int) bool {
		return part != ""
	})

	if len(filteredParts) == 0 {
		return ""
	}

	return strings.Join(filteredParts, ".")
}

func NewOperation(opType string, globalType OperationType, handler OperationHandler) Operation {
	return NewNamedOperation(opType, globalType, handler, "")
}

func NewNamedOperation(opType string, globalType OperationType, handler OperationHandler, name string) Operation {
	return &operation{
		opType:     opType,
		globalType: globalType,
		handler:    handler,
		name:       name,
	}
}

func NewStoreOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		StoreOperationName(protocol),
		OpTypeStore,
		handler,
	)
}

func NewRetrieveOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		RetrieveOperationName(protocol),
		OpTypeRetrieve,
		handler,
	)
}

func NewPublishOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		PublishOperationName(protocol),
		OpTypePublish,
		handler,
	)
}

func NewUploadOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		UploadOperationName(protocol),
		OpTypeUpload,
		handler,
	)
}

func NewTUSUploadOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		TUSUploadOperationName(protocol),
		OpTypeUpload,
		handler,
	)
}

func NewPostUploadOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		postOperationName(protocol, OpTypeUpload),
		OpTypeUpload,
		handler,
	)
}

func NewScanOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		ScanOperationName(protocol),
		OpTypeScan,
		handler,
	)
}

func NewUnstoreOperation(protocol string, handler OperationHandler) Operation {
	return NewOperation(
		UnstoreOperationName(protocol),
		OpTypeUnstore,
		handler,
	)
}

// OperationFinder provides methods for finding operations by type
type OperationFinder interface {
	// FindOperationHandler finds an operation handler by full operation type (e.g. "ipfs.pin")
	FindOperationHandler(operationType string) (Operation, OperationHandler, error)
	// FindProtocolOperation finds an operation handler for a protocol operation
	FindProtocolOperation(operationType string) (Operation, OperationHandler, error)
	// FindPluginOperation finds an operation handler for a plugin operation
	FindPluginOperation(operationType string) (Operation, OperationHandler, error)
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

	// NewProgressTracker creates a progress tracker for this operation
	NewProgressTracker(requestID uint, mode ProgressMode, configFunc func(*ProgressTrackerConfig)) (*ProgressTracker, error)

	// GetProgressFromWorkflowData retrieves progress from workflow data
	GetProgressFromWorkflowData(requestID uint) (*ProgressUpdate, error)

	// GetStatusFromWorkflowData retrieves status from workflow data, using progress tracker data if available
	GetStatusFromWorkflowData(requestID uint, req *models.Request) (*RequestStatus, error)

	// NewDefaultProgressMessageProvider creates a message provider using the operation's display name
	// It automatically fetches the display name from core's OperationTypeDisplayNames mapping
	NewDefaultProgressMessageProvider(operationType OperationType) *DefaultProgressMessageProvider

	// NewSimpleProgressMessageProvider creates a simple message provider using the operation's display name
	// It automatically fetches the display name from core's OperationTypeDisplayNames mapping
	NewSimpleProgressMessageProvider(operationType OperationType) *SimpleProgressMessageProvider
}

// OperationHelperDefault is the default implementation of OperationHelper
type OperationHelperDefault struct {
	ctx          Context
	proto        string
	unmarshalTag string          // Tag to use for unmarshaling (default: "json")
	workflow     WorkflowService // Cached workflow service instance
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

// GetOperationTypeDisplayInfo returns the human-readable display information for a given operation type
func GetOperationTypeDisplayInfo(opType OperationType) (OperationTypeInfo, bool) {
	info, exists := OperationTypeDisplayNames[opType]
	return info, exists
}

// GetOperationDisplayName returns the human-readable display name for a given operation
func GetOperationDisplayName(op Operation) string {
	if op == nil {
		return ""
	}

	if displayOp, ok := op.(OperationDisplay); ok {
		return displayOp.DisplayName()
	}

	if info, ok := GetOperationTypeDisplayInfo(op.GlobalType()); ok && info.Name != "" {
		return info.Name
	}

	return op.Type()
}

// StartWorkflow starts a new workflow with the given name and options
func (h *OperationHelperDefault) StartWorkflow(name string, opts ...WorkflowOption) (*models.Request, error) {
	return h.workflow.StartWorkflow(h.ctx, name, opts...)
}

// NewProgressTracker creates a progress tracker for this operation
func (h *OperationHelperDefault) NewProgressTracker(requestID uint, mode ProgressMode, configFunc func(*ProgressTrackerConfig)) (*ProgressTracker, error) {
	cfg := ProgressTrackerConfig{
		Mode:            mode,
		RequestID:       requestID,
		WorkflowService: h.workflow,
		Logger:          h.Logger(),
	}

	if configFunc != nil {
		configFunc(&cfg)
	}

	return NewProgressTracker(cfg)
}

// NewDefaultProgressMessageProvider creates a message provider using the operation's display name
// It automatically fetches the display name from core's OperationTypeDisplayNames mapping
func (h *OperationHelperDefault) NewDefaultProgressMessageProvider(operationType OperationType) *DefaultProgressMessageProvider {
	operationName := string(operationType)

	// Try to get display name from mappings
	if info, exists := OperationTypeDisplayNames[operationType]; exists {
		return NewDefaultProgressMessageProvider(info.Name)
	}

	// Fallback to operation type string
	return NewDefaultProgressMessageProvider(operationName)
}

// NewSimpleProgressMessageProvider creates a simple message provider using the operation's display name
// It automatically fetches the display name from core's OperationTypeDisplayNames mapping
func (h *OperationHelperDefault) NewSimpleProgressMessageProvider(operationType OperationType) *SimpleProgressMessageProvider {
	operationName := string(operationType)

	// Try to get display name from mappings
	if info, exists := OperationTypeDisplayNames[operationType]; exists {
		return NewSimpleProgressMessageProvider(info.Name)
	}

	// Fallback to operation type string
	return NewSimpleProgressMessageProvider(operationName)
}

// NewMessageProviderForOperation creates a message provider using the full operation string
// It extracts the operation type and looks up the display name from OperationTypeDisplayNames
// For example: "lbry.retrieve" → "Retrieve", "ipfs.upload" → "Upload"
func NewMessageProviderForOperation(operation string, useSimpleProvider bool) ProgressMessageProvider {
	// Try to get operation display info
	if info, exists := getOperationDisplayInfo(operation); exists {
		if useSimpleProvider {
			return NewSimpleProgressMessageProvider(info.Name)
		}
		return NewDefaultProgressMessageProvider(info.Name)
	}

	// Fallback: extract last part of operation name and format it
	parts := strings.Split(operation, ".")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Convert underscore to space and capitalize
		displayName := strings.ReplaceAll(name, "_", " ")
		if len(displayName) > 0 {
			displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
		}

		if useSimpleProvider {
			return NewSimpleProgressMessageProvider(displayName)
		}
		return NewDefaultProgressMessageProvider(displayName)
	}

	// Ultimate fallback
	if useSimpleProvider {
		return NewSimpleProgressMessageProvider(ucFirst(progressProviderFallbackOperation))
	}
	return NewDefaultProgressMessageProvider(ucFirst(progressProviderFallbackOperation))
}

// GetProgressFromWorkflowData retrieves progress from workflow data
func (h *OperationHelperDefault) GetProgressFromWorkflowData(requestID uint) (*ProgressUpdate, error) {
	k, err := h.WorkflowData(requestID)
	if err != nil {
		return nil, err
	}

	update := &ProgressUpdate{
		ProgressPercent: k.Float64("progress_percent"),
		StepName:        k.String("step_name"),
		StepProgress:    k.Float64("step_progress"),
		Message:         k.String("message"),
	}

	if updatedAtStr := k.String("updated_at"); updatedAtStr != "" {
		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err == nil {
			update.UpdatedAt = updatedAt
		}
	} else {
		update.UpdatedAt = time.Now()
	}

	return update, nil
}

// GetStatusFromWorkflowData retrieves status from workflow data, using progress tracker data if available
func (h *OperationHelperDefault) GetStatusFromWorkflowData(requestID uint, req *models.Request) (*RequestStatus, error) {
	status := &RequestStatus{
		State:     req.Status,
		UpdatedAt: time.Now(),
	}

	// Try to get progress from workflow data
	progress, err := h.GetProgressFromWorkflowData(requestID)
	if err == nil {
		// Use progress data if available (even if 0%)
		status.ProgressPercent = progress.ProgressPercent
		status.UpdatedAt = progress.UpdatedAt

		// Use progress message from tracker if available
		// This preserves step-specific messages like "Storing SD blob locally"
		if progress.Message != "" {
			status.Message = progress.Message
		} else {
			// Fall back to operation-aware message
			status.Message = h.getOperationAwareMessage(req.Status, req.Operation)
		}

		// If progress is > 0, the operation is definitely processing
		// Preserve terminal states (Failed/Completed) regardless of progress
		if progress.ProgressPercent > 0 && status.State != models.RequestStatusCompleted && status.State != models.RequestStatusFailed {
			// Log status forcing — this is the mechanism that keeps operations
			// stuck in "Processing" when CompleteRequest is never called.
			// At debug level to avoid noise in normal operation, but
			// invaluable for diagnosing stuck operations.
			h.Logger().Debug("forcing status to Processing due to progress > 0",
				zap.Uint("requestID", requestID),
				zap.Float64("progressPercent", progress.ProgressPercent),
				zap.String("dbStatus", string(req.Status)),
				zap.String("forcedStatus", string(models.RequestStatusProcessing)))
			status.State = models.RequestStatusProcessing
		}
	} else {
		// Fall back to operation-aware status messages
		status.Message = h.getOperationAwareMessage(req.Status, req.Operation)

		// Set default progress based on status
		switch req.Status {
		case models.RequestStatusPending:
			status.ProgressPercent = 0
		case models.RequestStatusProcessing:
			status.ProgressPercent = 10 // Default processing progress
		case models.RequestStatusCompleted:
			status.ProgressPercent = 100
		case models.RequestStatusFailed:
			status.ProgressPercent = 0
		default:
			status.ProgressPercent = 0
		}
	}

	return status, nil
}

// getOperationAwareMessage generates a human-readable message based on status and operation type
// Uses the existing OperationTypeDisplayNames mappings when available
func (h *OperationHelperDefault) getOperationAwareMessage(status models.RequestStatusType, operation string) string {
	// Try to get operation display info from global mappings
	opDisplay, exists := getOperationDisplayInfo(operation)
	if exists {
		// Use the operation's display name
		opName := opDisplay.Name

		// Generate contextual message based on status
		switch status {
		case models.RequestStatusPending:
			return fmt.Sprintf("Waiting to start %s", opName)
		case models.RequestStatusProcessing:
			return fmt.Sprintf("Processing %s", opName)
		case models.RequestStatusCompleted:
			return fmt.Sprintf("%s completed", opName)
		case models.RequestStatusFailed:
			return fmt.Sprintf("%s failed", opName)
		case models.RequestStatusDuplicate:
			return GetDefaultStatusMessage(status)
		default:
			return fmt.Sprintf("%s in progress", opName)
		}
	}

	// Fallback to generic messages if operation not found in mappings
	switch status {
	case models.RequestStatusProcessing:
		return fmt.Sprintf("Processing %s", operation)
	default:
		// All other statuses use the default message
		return GetDefaultStatusMessage(status)
	}
}

// getOperationDisplayInfo gets display info for an operation string
// First tries global type mappings, then falls back to extracting the last part
func getOperationDisplayInfo(operation string) (OperationTypeInfo, bool) {
	if operation == "" {
		return OperationTypeInfo{}, false
	}

	// Try to match against global operation types
	for opType, info := range OperationTypeDisplayNames {
		if strings.HasSuffix(operation, "."+string(opType)) {
			return info, true
		}
	}

	// Try to match against post_upload and tus_upload variants
	if strings.HasSuffix(operation, ".post_upload") {
		if info, exists := OperationTypeDisplayNames[OpTypeUpload]; exists {
			return info, true
		}
	}
	if strings.HasSuffix(operation, ".tus_upload") {
		if info, exists := OperationTypeDisplayNames[OpTypeUpload]; exists {
			return info, true
		}
	}

	return OperationTypeInfo{}, false
}

// ucFirst converts the first character of a string to uppercase
func ucFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
