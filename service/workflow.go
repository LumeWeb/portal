package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/datatypes"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.WorkflowService = (*WorkflowCoordinatorDefault)(nil)
var _ core.Cronable = (*WorkflowCoordinatorDefault)(nil)

var (
	noRetryPolicy = &core.RetryPolicy{MaxRetries: 0}
)

// Register service
func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.WORKFLOW_SERVICE,
		Factory: NewWorkflowCoordinator,
		Depends: []string{core.REQUEST_SERVICE, core.CRON_SERVICE},
	})
}

// TUSUploadStep creates a workflow step for TUS upload that can be added to any workflow
func TUSUploadStep(failureBehavior core.FailureBehavior) core.OperationStep {
	return core.OperationStep{
		Operation:       models.RequestOperationTusUpload,
		FailureBehavior: failureBehavior,
	}
}

// WorkflowMetadata stored in request.Metadata JSON field
type WorkflowMetadata struct {
	WorkflowName  string `json:"workflow_name"`
	CurrentStep   int    `json:"current_step"`
	TotalSteps    int    `json:"total_steps"`
	NextRequestID uint   `json:"next_request_id,omitempty"`
	PrevRequestID uint   `json:"prev_request_id,omitempty"`
	StartedAt     int64  `json:"started_at"`
	InitialData   any    `json:"initial_data"`
}

// WorkflowCoordinatorDefault implements the WorkflowCoordinator interface
type WorkflowCoordinatorDefault struct {
	ctx         core.Context
	logger      *core.Logger
	requestSvc  core.RequestService
	cronService core.CronService
	db          *gorm.DB
	workflows   map[string]*core.WorkflowDefinition
	workflowsMu sync.RWMutex
}

func (w *WorkflowCoordinatorDefault) RegisterTasks(cron core.CronService) error {
	err := cron.RegisterJobType(workflowStepExecutorJobType, func() (core.CronJob, error) {
		return newWorkflowStepExecutorJob(), nil
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) ScheduleJobs(_ core.CronService) error {
	return nil
}

// NewWorkflowCoordinator creates a new workflow coordinator
func NewWorkflowCoordinator() (core.Service, []core.ContextBuilderOption, error) {
	coordinator := &WorkflowCoordinatorDefault{
		workflows: make(map[string]*core.WorkflowDefinition),
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			coordinator.ctx = ctx
			coordinator.logger = ctx.ServiceLogger(coordinator)
			coordinator.requestSvc = core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
			coordinator.cronService = core.GetService[core.CronService](ctx, core.CRON_SERVICE)
			coordinator.db = ctx.DB()

			coordinator.cronService.RegisterEntity(coordinator)
			return nil
		}),
	)

	return coordinator, opts, nil
}

// ID returns the service ID
func (w *WorkflowCoordinatorDefault) ID() string {
	return core.WORKFLOW_SERVICE
}

// RegisterWorkflow registers a new workflow
func (w *WorkflowCoordinatorDefault) RegisterWorkflow(name string, steps []core.OperationStep) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; exists {
		return fmt.Errorf("workflow '%s' already exists", name)
	}

	// Ensure all steps have handlers
	for i := range steps {
		if steps[i].Handler == nil {
			return fmt.Errorf("step %d has nil handler", i)
		}
	}

	w.workflows[name] = &core.WorkflowDefinition{
		Name:  name,
		Steps: steps,
	}

	w.logger.Debug("Registered workflow",
		zap.String("name", name),
		zap.Int("steps", len(steps)))

	return nil
}

// GetWorkflow returns a workflow by name
func (w *WorkflowCoordinatorDefault) GetWorkflow(name string) (*core.WorkflowDefinition, error) {
	w.workflowsMu.RLock()
	defer w.workflowsMu.RUnlock()

	wf, exists := w.workflows[name]
	if !exists {
		return nil, fmt.Errorf("workflow '%s' not found", name)
	}

	return wf, nil
}

// ListWorkflows returns all registered workflow names
func (w *WorkflowCoordinatorDefault) ListWorkflows() []string {
	w.workflowsMu.RLock()
	defer w.workflowsMu.RUnlock()

	names := make([]string, 0, len(w.workflows))
	for name := range w.workflows {
		names = append(names, name)
	}

	return names
}

// StartWorkflow starts a new workflow instance
func (w *WorkflowCoordinatorDefault) StartWorkflow(ctx context.Context, name string, initialData any) (*models.Request, error) {
	// Get workflow
	workflow, err := w.GetWorkflow(name)
	if err != nil {
		return nil, err
	}

	if len(workflow.Steps) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	// First step
	firstStep := workflow.Steps[0]

	dataIsRequest := false
	var modelRequest *models.Request

	if _, ok := initialData.(*models.Request); ok {
		dataIsRequest = true
		modelRequest = initialData.(*models.Request)
	}

	// Create metadata for first step
	metadata := WorkflowMetadata{
		WorkflowName: workflow.Name,
		CurrentStep:  0,
		TotalSteps:   len(workflow.Steps),
		StartedAt:    time.Now().Unix(),
	}

	if !dataIsRequest {
		metadata.InitialData = initialData
	}

	// Serialize metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// Create initial request
	req := &models.Request{
		Operation: firstStep.Operation,
		Status:    models.RequestStatusPending,
		Metadata:  datatypes.JSON(metadataJSON),
	}

	// If initialData is a Request, copy its fields
	if dataIsRequest {
		req.Protocol = modelRequest.Protocol
		req.UserID = modelRequest.UserID
		req.SourceIP = modelRequest.SourceIP
		req.Hash = modelRequest.Hash
		req.CIDType = modelRequest.CIDType
		req.UploadHash = modelRequest.UploadHash
		req.UploadHashCIDType = modelRequest.UploadHashCIDType
		req.Size = modelRequest.Size
		req.MimeType = modelRequest.MimeType
	}

	// Validate the request
	if err := firstStep.Handler.ValidateRequest(ctx, req); err != nil {
		return nil, err
	}

	// Create the request
	createdReq, err := w.requestSvc.CreateRequest(ctx, req, nil)
	if err != nil {
		return nil, err
	}

	w.logger.Info("Started workflow",
		zap.String("workflow", workflow.Name),
		zap.Uint("requestID", createdReq.ID))

	// Auto-trigger first step if configured
	if workflow.AutoTriggerFirstStep {
		firstStep := workflow.Steps[0]
		if firstStep.DelegateToCron {
			if err := w.dispatchToCron(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to dispatch first step to cron",
					zap.String("workflow", workflow.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil // Return request even if cron dispatch fails
			}
		} else {
			if err := w.ExecuteWorkflowStep(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to execute first step",
					zap.String("workflow", workflow.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil // Return request even if execution fails
			}
		}
	}

	return createdReq, nil
}

// CompleteWorkflowStep completes the current step and advances to the next
func (w *WorkflowCoordinatorDefault) CompleteWorkflowStep(ctx context.Context, requestID uint) error {
	// Get current request
	currentReq, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(currentReq.Metadata, &metadata); err != nil {
		return fmt.Errorf("invalid workflow meta %w", err)
	}

	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	// Mark current step complete
	if err := w.requestSvc.CompleteRequest(ctx, requestID); err != nil {
		return err
	}

	// Check if we're done with the workflow
	if metadata.CurrentStep >= len(workflow.Steps)-1 {
		w.logger.Info("Workflow completed",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Create next step
	nextStepIdx := metadata.CurrentStep + 1
	nextStep := workflow.Steps[nextStepIdx]

	// Update metadata for next step
	metadata.CurrentStep = nextStepIdx
	metadata.PrevRequestID = requestID

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Create next request - copying relevant fields from current request
	nextReq := &models.Request{
		Operation:         nextStep.Operation,
		Protocol:          currentReq.Protocol,
		Status:            models.RequestStatusPending,
		UserID:            currentReq.UserID,
		SourceIP:          currentReq.SourceIP,
		Hash:              currentReq.Hash,
		CIDType:           currentReq.CIDType,
		UploadHash:        currentReq.UploadHash,
		UploadHashCIDType: currentReq.UploadHashCIDType,
		Size:              currentReq.Size,
		MimeType:          currentReq.MimeType,
		Metadata:          datatypes.JSON(metadataJSON),
	}

	// Create next request - fix CreateRequest call
	createdReq, err := w.requestSvc.CreateRequest(ctx, nextReq, nil)
	if err != nil {
		return err
	}

	// Update current request's metadata with next ID
	metadata.NextRequestID = createdReq.ID
	metadataJSON, _ = json.Marshal(metadata)
	currentReq.Metadata = metadataJSON

	// Update current request's metadata with next ID and save it
	currentReq.Metadata = metadataJSON
	if err := w.requestSvc.UpdateRequest(ctx, currentReq); err != nil {
		return fmt.Errorf("failed to update current request metadata: %w", err)
	}

	if nextStep.DelegateToCron {
		if err := w.dispatchToCron(ctx, createdReq.ID); err != nil {
			return err
		}
		w.logger.Info("Delegated workflow step to cron",
			zap.String("workflow", metadata.WorkflowName),
			zap.Int("step", nextStepIdx),
			zap.Uint("nextRequestID", createdReq.ID))
	} else {
		// Execute the next step directly
		err = w.ExecuteWorkflowStep(ctx, createdReq.ID)
		if err != nil {
			return err
		}
		w.logger.Info("Advanced workflow to next step",
			zap.String("workflow", metadata.WorkflowName),
			zap.Int("step", nextStepIdx),
			zap.Uint("nextRequestID", createdReq.ID))
	}

	return nil
}

// FailWorkflowStep handles a step failure
func (w *WorkflowCoordinatorDefault) FailWorkflowStep(ctx context.Context, requestID uint, reason string) error {
	// Get current request
	currentReq, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(currentReq.Metadata, &metadata); err != nil {
		return fmt.Errorf("invalid workflow meta %w", err)
	}

	// Get workflow and current step
	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStep := workflow.Steps[metadata.CurrentStep]

	// Mark current step failed using RequestService
	if err = w.requestSvc.FailRequest(ctx, currentReq.ID, reason); err != nil {
		return err
	}

	// Handle according to failure behavior
	switch currentStep.FailureBehavior {
	case core.FailWorkflow:
		w.logger.Info("Workflow failed",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID),
			zap.String("reason", reason))
		return nil

	case core.ContinueWorkflow:
		// Continue to next step despite failure
		return w.CompleteWorkflowStep(ctx, requestID)

	case core.RetryStep:
		// Schedule retry with backoff
		return w.scheduleRetry(ctx, requestID)
	}

	return nil
}

// GetWorkflowStatus returns the current status of a workflow
func (w *WorkflowCoordinatorDefault) GetWorkflowStatus(ctx context.Context, requestID uint) (*core.WorkflowStatus, error) {
	// Get request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Parse metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("invalid workflow meta %w", err)
	}

	// Build status
	status := &core.WorkflowStatus{
		WorkflowName:  metadata.WorkflowName,
		CurrentStep:   metadata.CurrentStep,
		TotalSteps:    metadata.TotalSteps,
		Status:        string(req.Status),
		CurrentStepID: req.ID,
		StartedAt:     time.Unix(metadata.StartedAt, 0),
		UpdatedAt:     req.UpdatedAt,
	}

	// Calculate progress
	if metadata.TotalSteps > 0 {
		status.Progress = float64(metadata.CurrentStep) / float64(metadata.TotalSteps) * 100

		// For completed requests, report 100% for this step
		if req.Status == models.RequestStatusCompleted {
			status.Progress = float64(metadata.CurrentStep+1) / float64(metadata.TotalSteps) * 100
		}
	}

	// Get previous steps recursively
	previousSteps := []uint{}
	prevID := metadata.PrevRequestID
	for prevID != 0 {
		previousSteps = append(previousSteps, prevID)

		prevReq, err := w.requestSvc.GetRequest(ctx, prevID)
		if err != nil {
			w.logger.Warn("Previous request not found in workflow chain",
				zap.Uint("requestID", prevID),
				zap.Error(err))
			break
		}

		var prevMetadata WorkflowMetadata
		if err := json.Unmarshal(prevReq.Metadata, &prevMetadata); err != nil {
			w.logger.Warn("Failed to parse metadata for previous request",
				zap.Uint("requestID", prevID),
				zap.Error(err))
			break
		}

		prevID = prevMetadata.PrevRequestID
	}

	status.PreviousSteps = previousSteps

	return status, nil
}

// ExecuteWorkflowStep executes the operation handler for a workflow step
func (w *WorkflowCoordinatorDefault) ExecuteWorkflowStep(ctx context.Context, requestID uint) error {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Get workflow metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return fmt.Errorf("invalid workflow meta %w", err)
	}

	// Get workflow and current step
	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}
	currentStep := workflow.Steps[metadata.CurrentStep]

	// Execute the operation handler
	err = currentStep.Handler.Execute(ctx, req)
	if err != nil {
		// Fail the workflow step
		return w.FailWorkflowStep(ctx, req.ID, err.Error())
	}

	// Complete the workflow step
	return w.CompleteWorkflowStep(ctx, req.ID)
}

// CanTransition checks if a workflow step can be transitioned from its current state
func (w *WorkflowCoordinatorDefault) CanTransition(ctx context.Context, requestID uint) (bool, error) {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return false, fmt.Errorf("failed to get request: %w", err)
	}

	// Check if the request is already completed or failed
	if req.Status == models.RequestStatusCompleted || req.Status == models.RequestStatusFailed {
		return false, nil
	}

	return true, nil
}

// GetWorkflowStepInfo returns information about a specific workflow step
func (w *WorkflowCoordinatorDefault) GetWorkflowStepInfo(ctx context.Context, requestID uint) (*core.WorkflowStepInfo, error) {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	// Parse metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("invalid workflow meta %w", err)
	}

	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return nil, err
	}

	currentStep := workflow.Steps[metadata.CurrentStep]

	return &core.WorkflowStepInfo{
		Operation:       currentStep.Operation,
		FailureBehavior: currentStep.FailureBehavior,
		Status:          string(req.Status),
	}, nil
}

// scheduleRetry schedules a retry for a failed step
func (w *WorkflowCoordinatorDefault) dispatchToCron(ctx context.Context, requestID uint) error {
	// Create and register the workflow step executor job
	job, err := w.cronService.JobFactory().CreateJob(workflowStepExecutorJobType)
	if err != nil {
		return fmt.Errorf("failed to create job instance: %w", err)
	}

	job.SetArgs(requestID)

	if err := w.cronService.RegisterJob(job, noRetryPolicy); err != nil {
		return fmt.Errorf("failed to register workflow step job: %w", err)
	}
	return nil
}

func (w *WorkflowCoordinatorDefault) scheduleRetry(ctx context.Context, requestID uint) error {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// For now, just reset status to pending
	err = db.RetryableTransaction(w.ctx, w.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", req.ID).
			Updates(map[string]interface{}{
				"status": models.RequestStatusPending,
			})
	})

	return err
}
