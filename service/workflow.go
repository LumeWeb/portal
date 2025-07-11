package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
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

// copyRequestData copies fields from source request to target request
func (w *WorkflowCoordinatorDefault) copyRequestData(target, source *models.Request) {
	target.Protocol = source.Protocol
	target.UserID = source.UserID
	target.SourceIP = source.SourceIP
	target.Hash = source.Hash
	target.CIDType = source.CIDType
}

// handleRequestData processes request data options and updates the target request
func (w *WorkflowCoordinatorDefault) handleRequestData(options core.WorkflowOptions, target *models.Request) {
	if requestData := options.RequestData(); requestData != nil {
		switch rd := requestData.(type) {
		case *models.Request:
			w.copyRequestData(target, rd)
		}
	}

	if sourceIP := options.SourceIP(); sourceIP != "" {
		target.SourceIP = sourceIP
	}

	if storageHash := options.StorageHash(); storageHash != nil {
		target.Hash = storageHash.Multihash()
		target.CIDType = storageHash.CIDType()
	}

	if userID := options.UserID(); userID > 0 {
		target.UserID = &userID
	}

	if protocol := options.Protocol(); protocol != "" {
		target.Protocol = protocol
	}
}

// processWorkflowOptions handles workflow options and updates metadata accordingly
// Returns the processed options and marshaled metadata
func (w *WorkflowCoordinatorDefault) processWorkflowOptions(opts []core.WorkflowOption, metadata *WorkflowMetadata) (core.WorkflowOptions, []byte, error) {
	// Initialize options with defaults
	options := core.NewWorkflowOptions()

	// Apply all provided options
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, nil, err
		}
	}

	// Handle metadata merging
	if metadata != nil && string(metadata.Data) != "" {
		if err := options.MergeJSON(string(metadata.Data)); err != nil {
			return nil, nil, err
		}
	}

	// Merge new data if provided
	if data := options.Data(); data != nil {
		if err := options.MergeData(data); err != nil {
			return nil, nil, err
		}
	}

	// Serialize the final koanf data for storage
	dataBytes, err := options.MarshalData()
	if err != nil {
		return nil, nil, err
	}
	return options, dataBytes, nil
}

// WorkflowMetadata stored in request.Metadata JSON field
type WorkflowMetadata struct {
	WorkflowName  string         `json:"workflow_name"`
	CurrentStep   int            `json:"current_step"`
	TotalSteps    int            `json:"total_steps"`
	NextRequestID uint           `json:"next_request_id,omitempty"`
	PrevRequestID uint           `json:"prev_request_id,omitempty"`
	StartedAt     int64          `json:"started_at"`
	Data          datatypes.JSON `json:"data"`
}

// WorkflowCoordinatorDefault implements the WorkflowCoordinator interface
type WorkflowCoordinatorDefault struct {
	ctx         core.Context
	logger      *core.Logger
	requestSvc  core.RequestService
	cronService core.CronService
	db          *gorm.DB
	workflows   map[string]*core.WorkflowDefinition
	disabled    map[string]bool
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
		disabled:  make(map[string]bool),
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
func (w *WorkflowCoordinatorDefault) RegisterWorkflow(name string, steps []core.OperationStep, autoTriggerFirstStep bool) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; exists {
		return fmt.Errorf("workflow '%s' already exists", name)
	}

	w.workflows[name] = &core.WorkflowDefinition{
		Name:                 name,
		Steps:                steps,
		AutoTriggerFirstStep: autoTriggerFirstStep,
	}

	w.logger.Debug("Registered workflow",
		zap.String("name", name),
		zap.Int("steps", len(steps)),
		zap.Bool("autoTriggerFirstStep", autoTriggerFirstStep))

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

func (w *WorkflowCoordinatorDefault) DisableWorkflow(name string) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; !exists {
		return fmt.Errorf("workflow '%s' not found", name)
	}

	w.disabled[name] = true
	w.logger.Debug("Disabled workflow", zap.String("name", name))
	return nil
}

func (w *WorkflowCoordinatorDefault) EnableWorkflow(name string) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; !exists {
		return fmt.Errorf("workflow '%s' not found", name)
	}

	delete(w.disabled, name)
	w.logger.Debug("Enabled workflow", zap.String("name", name))
	return nil
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
func (w *WorkflowCoordinatorDefault) StartWorkflow(ctx context.Context, name string, opts ...core.WorkflowOption) (*models.Request, error) {
	// Get workflow
	workflow, err := w.GetWorkflow(name)
	if err != nil {
		return nil, err
	}

	w.workflowsMu.RLock()
	disabled := w.disabled[name]
	w.workflowsMu.RUnlock()

	if disabled {
		w.logger.Warn("Attempted to start disabled workflow",
			zap.String("workflow", name))
		return nil, nil
	}

	if len(workflow.Steps) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	// First step
	firstStep := workflow.Steps[0]

	// Create and process metadata for first step
	metadata := WorkflowMetadata{
		WorkflowName:  workflow.Name,
		CurrentStep:   0,
		TotalSteps:    len(workflow.Steps),
		StartedAt:     time.Now().Unix(),
		PrevRequestID: 0, // Explicitly initialize
		NextRequestID: 0, // Explicitly initialize
	}

	processedOpts, wfMetadataJSON, err := w.processWorkflowOptions(opts, &metadata)
	if err != nil {
		return nil, err
	}

	metadata.Data = wfMetadataJSON

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

	w.handleRequestData(processedOpts, req)

	// Validate the request using request service
	if err := w.requestSvc.ValidateRequest(ctx, req); err != nil {
		return nil, err
	}

	// Create the request
	createdReq, err := w.requestSvc.CreateRequest(ctx, req, processedOpts.RequestData())
	if err != nil {
		return nil, err
	}

	w.logger.Info("Started workflow",
		zap.String("workflow", workflow.Name),
		zap.Uint("requestID", createdReq.ID))

	// Auto-trigger first step if configured
	if workflow.AutoTriggerFirstStep {
		firstStep := workflow.Steps[0]
		if firstStep.Foreground {
			if err := w.ExecuteWorkflowStep(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to execute first step",
					zap.String("workflow", workflow.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil // Return request even if execution fails
			}
		} else {
			if err := w.dispatchToCron(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to dispatch first step to cron",
					zap.String("workflow", workflow.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil // Return request even if cron dispatch fails
			}
		}
	}

	return createdReq, nil
}

// CompleteWorkflowStep completes the current step and advances to the next
func (w *WorkflowCoordinatorDefault) isDisabled(name string) bool {
	w.workflowsMu.RLock()
	defer w.workflowsMu.RUnlock()
	return w.disabled[name]
}

func (w *WorkflowCoordinatorDefault) parseWorkflowMetadata(metadataJSON datatypes.JSON) (WorkflowMetadata, error) {
	var metadata WorkflowMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return metadata, fmt.Errorf("invalid workflow meta %w", err)
	}
	return metadata, nil
}

func (w *WorkflowCoordinatorDefault) CompleteWorkflowStep(ctx context.Context, requestID uint, opts ...core.WorkflowOption) error {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(req.Metadata)
	if err != nil {
		return err
	}

	nextMetadata := metadata

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to complete step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Mark current step as completed
	if err := w.requestSvc.CompleteRequest(ctx, requestID); err != nil {
		return err
	}

	// Get workflow definition
	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	// Check if workflow is complete
	if metadata.CurrentStep >= len(workflow.Steps)-1 {
		w.logger.Info("Workflow completed",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return w.CleanupWorkflow(ctx, requestID)
	}

	// Prepare next step
	nextStep := workflow.Steps[metadata.CurrentStep+1]

	// Process workflow options and metadata
	processedOpts, wfMetadataJSON, err := w.processWorkflowOptions(opts, &metadata)
	if err != nil {
		return err
	}

	// Update metadata with next request ID and workflow data
	nextMetadata.PrevRequestID = requestID
	nextMetadata.Data = wfMetadataJSON

	// Create next request
	nextReq := &models.Request{
		Operation: nextStep.Operation,
		Protocol:  req.Protocol,
		Status:    models.RequestStatusPending,
		UserID:    req.UserID,
		SourceIP:  req.SourceIP,
		Hash:      req.Hash,
		CIDType:   req.CIDType,
	}

	w.handleRequestData(processedOpts, nextReq)

	// Update current request's metadata with next request ID
	nextMetadata.CurrentStep++
	metadataJSON, err := json.Marshal(nextMetadata)
	if err != nil {
		return err
	}

	nextReq.Metadata = metadataJSON

	// Create the next request
	createdReq, err := w.requestSvc.CreateRequest(ctx, nextReq, processedOpts.RequestData())
	if err != nil {
		return err
	}

	req, err = w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	metadata.NextRequestID = createdReq.ID

	metadataJSON, err = json.Marshal(metadata)
	if err != nil {
		return err
	}
	// Persist updated metadata on the current request
	req.Metadata = metadataJSON
	if err = w.requestSvc.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("failed to update current request metadata: %w", err)
	}

	// Execute or dispatch next step
	if nextStep.Foreground {
		return w.ExecuteWorkflowStep(ctx, createdReq.ID)
	}
	return w.dispatchToCron(ctx, createdReq.ID)
}

// FailWorkflowStep handles a step failure
func (w *WorkflowCoordinatorDefault) FailWorkflowStep(ctx context.Context, requestID uint, reason string) error {
	// Get current request
	currentReq, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(currentReq.Metadata)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to fail step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
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
	req, err := w.requestSvc.GetRequestWithDeleted(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(req.Metadata)
	if err != nil {
		return nil, err
	}

	// Get detailed request status
	reqStatus, err := w.requestSvc.GetRequestStatus(ctx, requestID, true)
	if err != nil {
		return nil, err
	}

	// Build status
	status := &core.WorkflowStatus{
		WorkflowName:  metadata.WorkflowName,
		CurrentStep:   metadata.CurrentStep,
		TotalSteps:    metadata.TotalSteps,
		Status:        reqStatus.State,
		CurrentStepID: req.ID,
		StartedAt:     time.Unix(metadata.StartedAt, 0),
		UpdatedAt:     reqStatus.UpdatedAt,
	}

	// Calculate unified weighted progress
	if metadata.TotalSteps > 0 {
		// Step progress contributes 50% weight
		stepProgress := float64(metadata.CurrentStep) / float64(metadata.TotalSteps)

		// Current step's internal progress contributes 50% weight
		stepInternalProgress := reqStatus.ProgressPercent / 100.0

		// Combine weighted progress
		unifiedProgress := (stepProgress*0.5 + stepInternalProgress*0.5) * 100
		status.Progress = unifiedProgress

		// For completed requests, report 100%
		if req.Status == models.RequestStatusCompleted {
			status.Progress = 100
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

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(req.Metadata)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to execute step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Get workflow and current step
	workflow, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}
	currentStep := workflow.Steps[metadata.CurrentStep]

	// Delegate execution to RequestService which will lookup the handler
	err = w.requestSvc.ExecuteRequest(ctx, requestID)
	if err != nil {
		// Handle failure according to step's behavior
		if currentStep.FailureBehavior == core.FailWorkflow {
			return w.FailWorkflowStep(ctx, requestID, err.Error())
		}
		return err
	}

	// If execution succeeded, complete it
	return w.CompleteWorkflowStep(ctx, requestID)
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
	req, err := w.requestSvc.GetRequestWithDeleted(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(req.Metadata)
	if err != nil {
		return nil, err
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

func (w *WorkflowCoordinatorDefault) GetWorkflowMetadata(ctx context.Context, requestID uint) (*koanf.Koanf, error) {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Parse metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("invalid workflow meta %w", err)
	}

	k, err := w.jsonToKoanf(string(metadata.Data))
	if err != nil {
		return nil, err
	}

	return k, nil
}

func (w *WorkflowCoordinatorDefault) UpdateWorkflowData(ctx context.Context, requestID uint, data map[string]any) error {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse existing metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return fmt.Errorf("invalid workflow meta %w", err)
	}

	// Create workflow options and merge data
	opts := core.NewWorkflowOptions()
	if string(metadata.Data) != "" {
		if err := opts.MergeJSON(string(metadata.Data)); err != nil {
			return err
		}
	}
	if err := opts.MergeData(data); err != nil {
		return err
	}

	// Marshal back to JSON
	dataBytes, err := opts.MarshalData()
	if err != nil {
		return fmt.Errorf("failed to marshal workflow data: %w", err)
	}
	metadata.Data = dataBytes

	// Update request metadata
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow metadata: %w", err)
	}

	req.Metadata = metadataBytes
	return w.requestSvc.UpdateRequest(ctx, req)
}

func (w *WorkflowCoordinatorDefault) UpdateWorkflowDataStruct(ctx context.Context, requestID uint, data any, tag string) error {
	// Create workflow options and load struct data
	opts := core.NewWorkflowOptions()
	if err := opts.MergeStruct(data, tag); err != nil {
		return err
	}

	// Use the map version to update
	return w.UpdateWorkflowData(ctx, requestID, opts.Data())
}

// jsonToKoanf converts a JSON string to a koanf.Koanf instance
func (w *WorkflowCoordinatorDefault) jsonToKoanf(jsonStr string) (*koanf.Koanf, error) {
	k := koanf.New(".")
	if jsonStr == "" {
		return k, nil
	}

	err := k.Load(rawbytes.Provider([]byte(jsonStr)), kjson.Parser())
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (w *WorkflowCoordinatorDefault) ConvertRequestToWorkflow(ctx context.Context, requestID uint, workflowName string, startStep int, opts ...core.WorkflowOption) error {
	if w.isDisabled(workflowName) {
		w.logger.Warn("Attempted to convert request to disabled workflow",
			zap.String("workflow", workflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Get the existing request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Get the workflow definition
	workflow, err := w.GetWorkflow(workflowName)
	if err != nil {
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	if startStep < 0 || startStep >= len(workflow.Steps) {
		return fmt.Errorf("invalid start step: %d", startStep)
	}

	// Create initial metadata
	metadata := WorkflowMetadata{
		WorkflowName: workflow.Name,
		CurrentStep:  startStep,
		TotalSteps:   len(workflow.Steps),
		StartedAt:    time.Now().Unix(),
	}

	// Process workflow options and get metadata JSON
	processedOpts, wfMetadataJSON, err := w.processWorkflowOptions(opts, &metadata)
	if err != nil {
		return err
	}

	metadata.Data = wfMetadataJSON

	// Marshal the complete metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Update the request's metadata and operation
	req.Metadata = metadataJSON
	req.Operation = workflow.Steps[startStep].Operation

	w.handleRequestData(processedOpts, req)

	// Save the updated request
	if err := w.requestSvc.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) CleanupWorkflow(ctx context.Context, requestID uint) error {
	// Get the initial request
	initialReq, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get initial request: %w", err)
	}

	// Parse metadata
	metadata, err := w.parseWorkflowMetadata(initialReq.Metadata)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to cleanup disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Iterate through all requests in the workflow
	currentReqID := requestID
	for {
		// Delete the request through RequestService which handles cleanup
		err = w.requestSvc.DeleteRequest(ctx, currentReqID)
		if err != nil {
			w.logger.Warn("Failed to delete request during cleanup",
				zap.Uint("requestID", currentReqID),
				zap.Error(err))
		}

		// Get the request to check for previous IDs
		req, err := w.requestSvc.GetRequestWithDeleted(ctx, currentReqID)
		if err != nil {
			break // Stop if we can't get the request
		}

		// Parse metadata to find the previous request
		metadata, err = w.parseWorkflowMetadata(req.Metadata)
		if err != nil {
			w.logger.Warn("Failed to parse metadata during cleanup",
				zap.Uint("requestID", currentReqID),
				zap.Error(err))
			break
		}

		// Move to the previous request
		currentReqID = metadata.PrevRequestID

		// If there is no previous request, we are done
		if currentReqID == 0 {
			break
		}
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) FindWorkflowInstances(
	ctx context.Context,
	workflowName string,
	filter core.RequestFilter,
) ([]*core.WorkflowInstance, error) {
	if w.isDisabled(workflowName) {
		return nil, nil
	}

	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	// First find all requests belonging to this workflow
	var allRequests []*models.Request
	if err := db.RetryableTransaction(w.ctx, w.db, func(g *gorm.DB) *gorm.DB {
		return w.db.Model(&models.Request{}).
			Clauses(datatypes.JSONQuery("metadata").Equals(workflowName, "workflow_name")).
			Scopes(applyFilters(filter)).
			Unscoped().
			Find(&allRequests)
	}); err != nil {
		return nil, fmt.Errorf("failed to query workflow requests: %w", err)
	}

	// Group requests by workflow instance (using NextRequestID chain)
	workflowChains := make(map[uint][]*models.Request)
	for _, req := range allRequests {
		var metadata WorkflowMetadata
		if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
			continue
		}

		// Find the root request ID for this workflow chain
		rootID := req.ID
		if metadata.PrevRequestID != 0 {
			rootID = metadata.PrevRequestID
			// Follow the chain backwards to find the root
			for {
				prevReq, err := w.requestSvc.GetRequestWithDeleted(ctx, rootID)
				if err != nil || prevReq == nil {
					break
				}
				var prevMeta WorkflowMetadata
				if err := json.Unmarshal(prevReq.Metadata, &prevMeta); err != nil {
					break
				}
				if prevMeta.PrevRequestID == 0 {
					break
				}
				rootID = prevMeta.PrevRequestID
			}
		}

		workflowChains[rootID] = append(workflowChains[rootID], req)
	}

	// For each workflow chain, find the request at the final step
	results := make([]*core.WorkflowInstance, 0, len(workflowChains))
	for _, chain := range workflowChains {
		if len(chain) == 0 {
			continue
		}

		// Find the request with CurrentStep == TotalSteps-1
		var finalReq *models.Request
		for _, req := range chain {
			var meta WorkflowMetadata
			if err := json.Unmarshal(req.Metadata, &meta); err != nil {
				continue
			}
			if meta.CurrentStep == meta.TotalSteps-1 {
				finalReq = req
				break // Found the final request
			}
		}
		
		if finalReq == nil {
			continue
		}

		status, err := w.GetWorkflowStatus(ctx, finalReq.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get status for request %d: %w", finalReq.ID, err)
		}

		stepInfo, err := w.GetWorkflowStepInfo(ctx, finalReq.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get step info for request %d: %w", finalReq.ID, err)
		}

		results = append(results, &core.WorkflowInstance{
			Request:     finalReq,
			Status:      status,
			CurrentStep: stepInfo,
		})
	}

	return results, nil
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
