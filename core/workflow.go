package core

import (
	"context"
	"errors"
	"time"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/queryutil"
)

const WORKFLOW_SERVICE = "workflow"

// Workflow name constants for common workflows
const (
	PinWorkflowName       = "pin"
	UploadWorkflowName    = "upload"
	TUSUploadWorkflowName = "tus_upload"
)

// Predefined errors
var (
	ErrWorkflowHasNoSteps = errors.New("workflow has no steps")
)

// WorkflowCoordinator orchestrates multi-step operations across protocols
type WorkflowCoordinator interface {
	// Register a new workflow with specified steps
	RegisterWorkflow(name string, steps []OperationStep, autoTriggerFirstStep bool) error

	// Get a registered workflow by name
	GetWorkflow(name string) (*WorkflowDefinition, error)

	// List all registered workflows
	ListWorkflows() []string

	// Disable a workflow by name (primarily for testing)
	DisableWorkflow(name string) error

	// Enable a workflow by name (primarily for testing)
	EnableWorkflow(name string) error

	// Start a new workflow instance
	StartWorkflow(ctx context.Context, name string, opts ...WorkflowOption) (*models.Request, error)

	// Convert an existing request to a workflow
	ConvertRequestToWorkflow(ctx context.Context, requestID uint, workflowName string, startStep int, opts ...WorkflowOption) error

	// Advance workflow to next step with optional data
	CompleteWorkflowStep(ctx context.Context, requestID uint, opts ...WorkflowOption) error

	// Handle failure in workflow step
	// err must be non-nil - implementations should defensively handle nil values
	FailWorkflowStep(ctx context.Context, requestID uint, err error) error

	// Get current status of workflow
	GetWorkflowStatus(ctx context.Context, requestID uint) (*WorkflowStatus, error)

	// Cleanup all requests in a workflow
	CleanupWorkflow(ctx context.Context, requestID uint) error

	// DispatchWorkflowStep runs the current step inline when marked foreground, or schedules it via cron otherwise
	DispatchWorkflowStep(ctx context.Context, requestID uint) error
}

// WorkflowInstance represents a found workflow instance
type WorkflowInstance struct {
	Request     *models.Request
	Status      *WorkflowStatus
	CurrentStep *WorkflowStepInfo
}

type WorkflowService interface {
	Service
	WorkflowCoordinator

	// ExecuteWorkflowStep executes the operation handler for a workflow step
	ExecuteWorkflowStep(ctx context.Context, requestID uint) error

	// FindWorkflowInstances finds workflow instances matching the criteria
	FindWorkflowInstances(
		ctx context.Context,
		workflowName string,
		filter RequestFilter,
	) ([]*WorkflowInstance, error)

	// ListWorkflowInstances lists workflow instances with filtering, sorting and pagination
	ListWorkflowInstances(ctx context.Context, userID uint, filters []queryutil.CrudFilter, sorts []queryutil.Sort, pagination queryutil.Pagination) ([]*WorkflowInstance, int64, error)

	// GetWorkflowInstance gets a specific workflow instance by request ID
	GetWorkflowInstance(ctx context.Context, userID uint, requestID uint) (*WorkflowInstance, error)

	// CanTransition checks if a workflow step can be transitioned from its current state
	CanTransition(ctx context.Context, requestID uint) (bool, error)

	// GetWorkflowStepInfo returns information about a specific workflow step
	GetWorkflowStepInfo(ctx context.Context, requestID uint) (*WorkflowStepInfo, error)

	// GetWorkflowMetadata returns the workflow metadata for a request
	GetWorkflowMetadata(ctx context.Context, requestID uint) (*koanf.Koanf, error)

	// ListDistinctWorkflowFilters fetches distinct filter values for workflows
	// (statuses, operations, protocols) from the database
	ListDistinctWorkflowFilters(ctx context.Context, userID uint, additionalFilters []queryutil.CrudFilter) (map[string][]string, error)

	// UpdateWorkflowData updates workflow metadata with the provided data map
	UpdateWorkflowData(ctx context.Context, requestID uint, data map[string]any) error

	// UpdateWorkflowDataStruct updates workflow metadata with the provided struct
	// using the specified struct tag (e.g. "json")
	UpdateWorkflowDataStruct(ctx context.Context, requestID uint, data any, tag string) error
}

// WorkflowStepInfo provides information about a workflow step
type WorkflowStepInfo struct {
	Operation       string
	FailureBehavior FailureBehavior
	Status          models.RequestStatusType
}

// OperationStep defines a single step in a workflow
type OperationStep struct {
	// The operation identifier (e.g., "ipfs.upload" or "content.scan")
	Operation string

	// What to do if this step fails
	FailureBehavior FailureBehavior

	// Whether to run the operation in the active request directly or send to the cron system
	Foreground bool

	// Unique identifier for this step
	ID string
}

// FailureBehavior defines behavior when a step fails
type FailureBehavior int

const (
	FailWorkflow     FailureBehavior = iota // Fail entire workflow
	ContinueWorkflow                        // Continue to next step anyway
	RetryStep                               // Retry this step (with backoff)
)

// WorkflowOption configures workflow options and may return an error
type WorkflowOption func(WorkflowOptions) error

type WorkflowData = map[string]any

// WorkflowOptions defines the interface for workflow options
type WorkflowOptions interface {
	Data() WorkflowData
	SetData(data WorkflowData)
	RequestData() any
	SetRequestData(data any)
	SourceIP() string
	SetSourceIP(ip string)
	StorageHash() StorageHash
	SetStorageHash(hash StorageHash)
	UserID() uint
	SetUserID(id uint)
	Protocol() string
	SetProtocol(protocol string)
	MergeData(data WorkflowData) error
	MergeJSON(jsonData string) error
	MergeStruct(data any, tag string) error
	MarshalData() ([]byte, error)
	HasData() bool
	GetKoanf() (*koanf.Koanf, error)
}

// WorkflowOptionsDefault is the default implementation of WorkflowOptions
type WorkflowOptionsDefault struct {
	data        WorkflowData
	requestData any
	sourceIP    string
	storageHash StorageHash
	userID      uint
	protocol    string
	koanfCache  *koanf.Koanf
}

// GetWorkflowStatusDisplayInfo returns the human-readable display information for a given workflow status
func GetWorkflowStatusDisplayInfo(status models.RequestStatusType) (WorkflowStatusInfo, bool) {
	info, exists := WorkflowStatusDisplayNames[status]
	return info, exists
}

// NewWorkflowOptions creates a new WorkflowOptions instance with initialized koanf cache
func NewWorkflowOptions() WorkflowOptions {
	return &WorkflowOptionsDefault{
		koanfCache: koanf.New("."),
	}
}

func (o *WorkflowOptionsDefault) Data() WorkflowData {
	return o.data
}

func (o *WorkflowOptionsDefault) SetData(data WorkflowData) {
	o.data = data
}

func (o *WorkflowOptionsDefault) RequestData() any {
	return o.requestData
}

func (o *WorkflowOptionsDefault) SetRequestData(data any) {
	o.requestData = data
}

func (o *WorkflowOptionsDefault) SourceIP() string {
	return o.sourceIP
}

func (o *WorkflowOptionsDefault) SetSourceIP(ip string) {
	o.sourceIP = ip
}

func (o *WorkflowOptionsDefault) StorageHash() StorageHash {
	return o.storageHash
}

func (o *WorkflowOptionsDefault) SetStorageHash(hash StorageHash) {
	o.storageHash = hash
}

func (o *WorkflowOptionsDefault) UserID() uint {
	return o.userID
}

func (o *WorkflowOptionsDefault) SetUserID(id uint) {
	o.userID = id
}

func (o *WorkflowOptionsDefault) Protocol() string {
	return o.protocol
}

func (o *WorkflowOptionsDefault) SetProtocol(protocol string) {
	o.protocol = protocol
}

// MergeData merges new data into the cached koanf instance
func (o *WorkflowOptionsDefault) MergeData(data WorkflowData) error {
	if o.koanfCache == nil {
		o.koanfCache = koanf.New(".")
	}
	if err := o.koanfCache.Load(confmap.Provider(data, "."), nil); err != nil {
		return err
	}

	o.data = o.koanfCache.Raw()
	return nil
}

// MergeStruct merges struct data into the cached koanf instance using the specified tag
func (o *WorkflowOptionsDefault) MergeStruct(data any, tag string) error {
	if o.koanfCache == nil {
		o.koanfCache = koanf.New(".")
	}

	k := koanf.New(".")
	if err := k.Load(structs.Provider(data, tag), nil); err != nil {
		return err
	}

	if err := o.koanfCache.Merge(k); err != nil {
		return err
	}

	o.data = o.koanfCache.Raw()
	return nil
}

// MergeJSON merges raw JSON data into the cached koanf instance
func (o *WorkflowOptionsDefault) MergeJSON(jsonData string) error {
	if jsonData == "" {
		return nil
	}

	if o.koanfCache == nil {
		o.koanfCache = koanf.New(".")
	}

	// Create temporary koanf instance to parse JSON
	k := koanf.New(".")
	if err := k.Load(rawbytes.Provider([]byte(jsonData)), json.Parser()); err != nil {
		return err
	}

	// Merge the parsed data into our cache
	if err := o.koanfCache.Merge(k); err != nil {
		return err
	}

	o.data = o.koanfCache.Raw()
	return nil
}

// MarshalData marshals the workflow data to JSON
func (o *WorkflowOptionsDefault) MarshalData() ([]byte, error) {
	if o.koanfCache == nil {
		return []byte("{}"), nil
	}
	return o.koanfCache.Marshal(json.Parser())
}

// HasData checks if there is any data in the koanf cache
func (o *WorkflowOptionsDefault) HasData() bool {
	if o.koanfCache == nil {
		return false
	}
	return len(o.koanfCache.Keys()) > 0
}

// GetKoanf returns the cached koanf instance or creates a new one
func (o *WorkflowOptionsDefault) GetKoanf() (*koanf.Koanf, error) {
	if o.koanfCache == nil {
		o.koanfCache = koanf.New(".")
		if o.data != nil {
			if err := o.koanfCache.Load(confmap.Provider(o.data, "."), nil); err != nil {
				return nil, err
			}
		}
	}
	return o.koanfCache, nil
}

// WithWorkflowData returns a WorkflowOption that sets the initial data
func WithWorkflowData(data WorkflowData) WorkflowOption {
	return func(o WorkflowOptions) error {
		return o.MergeData(data)
	}
}

// WithWorkflowStructData returns a WorkflowOption that sets the initial data from a struct
// using the specified struct tag (e.g. "json"). The struct will be converted to a map
// and merged into the workflow data.
func WithWorkflowStructData(data any, tag string) WorkflowOption {
	return func(o WorkflowOptions) error {
		return o.MergeStruct(data, tag)
	}
}

// WithWorkflowRequestData returns a WorkflowOption that sets the request data
func WithWorkflowRequestData(data any) WorkflowOption {
	return func(o WorkflowOptions) error {
		o.SetRequestData(data)
		return nil
	}
}

// WithWorkflowSourceIP returns a WorkflowOption that sets the SourceIP
func WithWorkflowSourceIP(ip string) WorkflowOption {
	return func(o WorkflowOptions) error {
		o.SetSourceIP(ip)
		return nil
	}
}

// WithWorkflowStorageHash returns a WorkflowOption that sets the Hash and CIDType from a StorageHash
func WithWorkflowStorageHash(hash StorageHash) WorkflowOption {
	return func(o WorkflowOptions) error {
		o.SetStorageHash(hash)
		return nil
	}
}

// WithWorkflowUserID returns a WorkflowOption that sets the UserID
func WithWorkflowUserID(userID uint) WorkflowOption {
	return func(o WorkflowOptions) error {
		o.SetUserID(userID)
		return nil
	}
}

// WithWorkflowProtocol returns a WorkflowOption that sets the Protocol
func WithWorkflowProtocol(protocol string) WorkflowOption {
	return func(o WorkflowOptions) error {
		o.SetProtocol(protocol)
		return nil
	}
}

// WorkflowDefinition represents a registered workflow
type WorkflowDefinition struct {
	Name                 string
	Steps                []OperationStep
	AutoTriggerFirstStep bool // Whether to automatically trigger first step execution
}

// WorkflowStatusInfo contains human-readable display information for workflow status
type WorkflowStatusInfo struct {
	Name        string `json:"name"`         // Human-readable display name
	Description string `json:"description"`  // Detailed description of the status
}

// WorkflowStatusDisplayNames maps workflow status types to their human-readable display information
var WorkflowStatusDisplayNames = map[models.RequestStatusType]WorkflowStatusInfo{
	models.RequestStatusPending:   {Name: "Pending", Description: "Waiting to start"},
	models.RequestStatusProcessing: {Name: "Processing", Description: "Currently executing"},
	models.RequestStatusCompleted:  {Name: "Completed", Description: "Finished successfully"},
	models.RequestStatusFailed:     {Name: "Failed", Description: "Encountered an error"},
	models.RequestStatusDuplicate:  {Name: "Duplicate", Description: "Duplicate detected"},
}

// WorkflowStatus provides current status of a workflow instance
type WorkflowStatus struct {
	WorkflowName  string
	CurrentStep   int
	TotalSteps    int
	Status        models.RequestStatusType
	Progress      float64
	CurrentStepID uint
	PreviousSteps []uint
	StartedAt     time.Time
	UpdatedAt     time.Time
	Message       string
}

// NewRetryStep creates an OperationStep with RetryStep failure behavior.
// This is the most common failure behavior for operations that may transiently fail
// and should be retried with backoff.
func NewRetryStep(operation string) OperationStep {
	return OperationStep{
		Operation:       operation,
		FailureBehavior: RetryStep,
		ID:              operation,
	}
}

// NewContinueStep creates an OperationStep with ContinueWorkflow failure behavior.
// Use this for optional operations where failure should not block the workflow.
func NewContinueStep(operation string) OperationStep {
	return OperationStep{
		Operation:       operation,
		FailureBehavior: ContinueWorkflow,
		ID:              operation,
	}
}

// NewFailStep creates an OperationStep with FailWorkflow failure behavior.
// Use this for critical operations where any failure should halt the entire workflow.
func NewFailStep(operation string) OperationStep {
	return OperationStep{
		Operation:       operation,
		FailureBehavior: FailWorkflow,
		ID:              operation,
	}
}

// NewWorkflowDefinition creates a WorkflowDefinition with the given parameters.
// This is a convenience function for creating workflow definitions.
func NewWorkflowDefinition(name string, autoTriggerFirstStep bool, steps []OperationStep) WorkflowDefinition {
	return WorkflowDefinition{
		Name:                 name,
		Steps:                steps,
		AutoTriggerFirstStep: autoTriggerFirstStep,
	}
}

// NewPinWorkflow creates a standard pin workflow definition for a protocol.
// The workflow includes: retrieve, scan, store, and optionally publish steps.
// Additional steps can be appended by the plugin as needed.
func NewPinWorkflow(protocol string, includePublish bool) WorkflowDefinition {
	steps := []OperationStep{
		NewRetryStep(RetrieveOperationName(protocol)),
		NewRetryStep(ScanOperationName(protocol)),
		NewRetryStep(StoreOperationName(protocol)),
	}

	if includePublish {
		steps = append(steps, NewContinueStep(PublishOperationName(protocol)))
	}

	return NewWorkflowDefinition(PinWorkflowName, true, steps)
}

// NewUploadWorkflow creates a standard upload workflow definition for a protocol.
// The workflow includes: post-upload step.
// Additional steps can be appended by the plugin as needed.
func NewUploadWorkflow(protocol string) WorkflowDefinition {
	return NewWorkflowDefinition(
		UploadWorkflowName,
		true,
		[]OperationStep{
			NewRetryStep(PostUploadOperationName(protocol)),
		},
	)
}

// NewTUSUploadWorkflow creates a standard TUS upload workflow definition for a protocol.
// The workflow includes: TUS upload step.
// Additional steps can be appended by the plugin as needed.
func NewTUSUploadWorkflow(protocol string) WorkflowDefinition {
	return NewWorkflowDefinition(
		TUSUploadWorkflowName,
		true,
		[]OperationStep{
			NewRetryStep(TUSUploadOperationName(protocol)),
		},
	)
}
