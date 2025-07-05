package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"time"
)

const WORKFLOW_SERVICE = "workflow"

// WorkflowCoordinator orchestrates multi-step operations across protocols
type WorkflowCoordinator interface {
	// Register a new workflow with specified steps
	RegisterWorkflow(name string, steps []OperationStep) error

	// Get a registered workflow by name
	GetWorkflow(name string) (*WorkflowDefinition, error)

	// List all registered workflows
	ListWorkflows() []string

	// Start a new workflow instance
	StartWorkflow(ctx context.Context, name string, opts ...WorkflowOption) (*models.Request, error)

	// Advance workflow to next step with optional data
	CompleteWorkflowStep(ctx context.Context, requestID uint, opts ...WorkflowOption) error

	// Handle failure in workflow step
	FailWorkflowStep(ctx context.Context, requestID uint, reason string) error

	// Get current status of workflow
	GetWorkflowStatus(ctx context.Context, requestID uint) (*WorkflowStatus, error)
}

type WorkflowService interface {
	Service
	WorkflowCoordinator

	// ExecuteWorkflowStep executes the operation handler for a workflow step
	ExecuteWorkflowStep(ctx context.Context, requestID uint) error

	// CanTransition checks if a workflow step can be transitioned from its current state
	CanTransition(ctx context.Context, requestID uint) (bool, error)

	// GetWorkflowStepInfo returns information about a specific workflow step
	GetWorkflowStepInfo(ctx context.Context, requestID uint) (*WorkflowStepInfo, error)
}

// WorkflowStepInfo provides information about a workflow step
type WorkflowStepInfo struct {
	Operation       string
	FailureBehavior FailureBehavior
	Status          string
}

// OperationStep defines a single step in a workflow
type OperationStep struct {
	// The operation identifier (e.g., "ipfs.upload" or "content.scan")
	Operation string

	// The handler for this operation
	Handler OperationHandler

	// What to do if this step fails
	FailureBehavior FailureBehavior

	// Whether to delegate execution to cron system
	DelegateToCron bool
}

// FailureBehavior defines behavior when a step fails
type FailureBehavior int

const (
	FailWorkflow     FailureBehavior = iota // Fail entire workflow
	ContinueWorkflow                        // Continue to next step anyway
	RetryStep                               // Retry this step (with backoff)
)

// WorkflowOption configures workflow options
type WorkflowOption func(*WorkflowOptions)

// WorkflowOptions contains options for starting a workflow
type WorkflowOptions struct {
	InitialData any
	RequestData any
}

// WithWorkflowInitialData returns a WorkflowOption that sets the initial data
func WithWorkflowInitialData(data any) WorkflowOption {
	return func(o *WorkflowOptions) {
		o.InitialData = data
	}
}

// WithWorkflowRequestData returns a WorkflowOption that sets the request data
func WithWorkflowRequestData(data any) WorkflowOption {
	return func(o *WorkflowOptions) {
		o.RequestData = data
	}
}

// WorkflowDefinition represents a registered workflow
type WorkflowDefinition struct {
	Name                 string
	Steps                []OperationStep
	AutoTriggerFirstStep bool // Whether to automatically trigger first step execution
}

// WorkflowStatus provides current status of a workflow instance
type WorkflowStatus struct {
	WorkflowName  string
	CurrentStep   int
	TotalSteps    int
	Status        string
	Progress      float64
	CurrentStepID uint
	PreviousSteps []uint
	StartedAt     time.Time
	UpdatedAt     time.Time
}
