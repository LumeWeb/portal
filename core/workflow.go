package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"time"
)

// WorkflowCoordinator orchestrates multi-step operations across protocols
type WorkflowCoordinator interface {
	// Register a new workflow with specified steps
	RegisterWorkflow(name string, steps []OperationStep) error

	// Get a registered workflow by name
	GetWorkflow(name string) (*WorkflowDefinition, error)

	// List all registered workflows
	ListWorkflows() []string

	// Start a new workflow instance
	StartWorkflow(ctx context.Context, name string, initialData interface{}) (*models.Request, error)

	// Advance workflow to next step
	CompleteWorkflowStep(ctx context.Context, requestID uint) error

	// Handle failure in workflow step
	FailWorkflowStep(ctx context.Context, requestID uint, reason string) error

	// Get current status of workflow
	GetWorkflowStatus(ctx context.Context, requestID uint) (*WorkflowStatus, error)
}

// OperationStep defines a single step in a workflow
type OperationStep struct {
	// The operation identifier (e.g., "ipfs.upload" or "content.scan")
	Operation string

	// The handler for this operation
	Handler OperationHandler

	// What to do if this step fails
	FailureBehavior FailureBehavior
}

// FailureBehavior defines behavior when a step fails
type FailureBehavior int

const (
	FailWorkflow     FailureBehavior = iota // Fail entire workflow
	ContinueWorkflow                        // Continue to next step anyway
	RetryStep                               // Retry this step (with backoff)
)

// WorkflowDefinition represents a registered workflow
type WorkflowDefinition struct {
	Name  string
	Steps []OperationStep
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
