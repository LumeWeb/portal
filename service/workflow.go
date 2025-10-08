package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/internal/workflow"
	"go.lumeweb.com/queryutil"

	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"gorm.io/datatypes"

	"github.com/samber/lo"
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

// NewWorkflowError creates a new workflow error
func NewWorkflowError(key core.WorkflowErrorType, message string, err error) *core.Error {
	return core.NewWorkflowError(key, err)
}

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

// getRequestWithWorkflowMetadata retrieves a request and parses its workflow metadata
func (w *WorkflowCoordinatorDefault) getRequestWithWorkflowMetadata(ctx context.Context, requestID uint) (*models.Request, WorkflowMetadata, error) {
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return nil, WorkflowMetadata{}, err
	}

	metadata, err := w.parseWorkflowMetadata(req.Metadata)
	if err != nil {
		return nil, WorkflowMetadata{}, err
	}

	return req, metadata, nil
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
	if metadata != nil && len(metadata.Data) > 0 {
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
	CurrentStepID string         `json:"current_step_id"`
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
		return core.NewWorkflowError(core.ErrKeyWorkflowAlreadyExists, fmt.Errorf("workflow '%s' already exists", name))
	}

	if len(steps) == 0 {
		return core.ErrWorkflowHasNoSteps
	}

	// Ensure each step has an ID
	for i := range steps {
		if steps[i].ID == "" {
			steps[i].ID = fmt.Sprintf("step-%s-%d", steps[i].Operation, i)
		}
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
		return nil, core.NewWorkflowError(core.ErrKeyWorkflowNotFound, fmt.Errorf("workflow '%s' not found", name))
	}

	return wf, nil
}

func (w *WorkflowCoordinatorDefault) DisableWorkflow(name string) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; !exists {
		return core.NewWorkflowError(core.ErrKeyWorkflowNotFound, fmt.Errorf("workflow '%s' not found", name))
	}

	w.disabled[name] = true
	w.logger.Debug("Disabled workflow", zap.String("name", name))
	return nil
}

func (w *WorkflowCoordinatorDefault) EnableWorkflow(name string) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; !exists {
		return core.NewWorkflowError(core.ErrKeyWorkflowNotFound, fmt.Errorf("workflow '%s' not found", name))
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
	wf, err := w.GetWorkflow(name)
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

	if len(wf.Steps) == 0 {
		return nil, core.ErrWorkflowHasNoSteps
	}

	// First step
	firstStep := wf.Steps[0]

	// Create and process metadata for first step
	metadata := WorkflowMetadata{
		WorkflowName:  wf.Name,
		CurrentStepID: firstStep.ID,
		TotalSteps:    len(wf.Steps),
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
		zap.String("workflow", wf.Name),
		zap.Uint("requestID", createdReq.ID))

	// Auto-trigger first step if configured
	if wf.AutoTriggerFirstStep {
		firstStep = wf.Steps[0]
		if firstStep.Foreground {
			if err := w.ExecuteWorkflowStep(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to execute first step",
					zap.String("workflow", wf.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil // Return request even if execution fails
			}
			// After execution, complete the step to advance to next step
			if err := w.CompleteWorkflowStep(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to complete first step",
					zap.String("workflow", wf.Name),
					zap.Uint("requestID", createdReq.ID),
					zap.Error(err))
				return createdReq, nil
			}
		} else {
			if err := w.DispatchWorkflowStep(ctx, createdReq.ID); err != nil {
				w.logger.Error("Failed to dispatch first step to cron",
					zap.String("workflow", wf.Name),
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
		return metadata, core.NewWorkflowError(core.ErrKeyWorkflowMetadataInvalid, fmt.Errorf("invalid workflow meta %w", err))
	}
	return metadata, nil
}

func (w *WorkflowCoordinatorDefault) CompleteWorkflowStep(ctx context.Context, requestID uint, opts ...core.WorkflowOption) error {
	// Get current request and parse metadata
	req, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
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
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStepIndex, err := w.getStepIndexAndStep(wf, metadata.CurrentStepID)
	if err != nil {
		return err
	}

	// Check if workflow is complete
	if currentStepIndex >= len(wf.Steps)-1 {
		w.logger.Info("Workflow completed",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return w.CleanupWorkflow(ctx, requestID)
	}

	// Prepare next step
	nextStep := wf.Steps[currentStepIndex+1]

	// Process workflow options and metadata
	processedOpts, wfMetadataJSON, err := w.processWorkflowOptions(opts, &metadata)
	if err != nil {
		return err
	}

	// Update metadata with next request ID and workflow data
	nextMetadata.PrevRequestID = requestID
	nextMetadata.CurrentStepID = nextStep.ID
	// Validate and convert JSON data
	if len(wfMetadataJSON) == 0 {
		nextMetadata.Data = datatypes.JSON("{}") // Default to empty JSON object
	} else if !json.Valid(wfMetadataJSON) {
		return core.NewWorkflowError(core.ErrKeyWorkflowMetadataInvalid, fmt.Errorf("invalid JSON data for workflow metadata"))
	} else {
		nextMetadata.Data = datatypes.JSON(wfMetadataJSON)
	}

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
		if err = w.ExecuteWorkflowStep(ctx, createdReq.ID); err != nil {
			return err
		}
		return nil
	}
	return w.DispatchWorkflowStep(ctx, createdReq.ID)
}

// FailWorkflowStep handles a step failure
func (w *WorkflowCoordinatorDefault) FailWorkflowStep(ctx context.Context, requestID uint, reason string) error {
	// Get current request and parse metadata
	currentReq, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
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
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStep, err := w.getStepByID(wf, metadata.CurrentStepID)
	if err != nil {
		return err
	}

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
		err := w.scheduleRetry(ctx, requestID)
		if err != nil {
			return err
		}
		// Return a retried error to indicate this was a retry
		return core.NewWorkflowError(core.ErrKeyWorkflowStepRetried, nil)
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

	currentStepIndex, err := w.getCurrentStepIndex(metadata)
	if err != nil {
		return nil, err
	}

	// Sanitize/clamp user-facing message (avoid leaking long internals)
	msg := reqStatus.Message
	if mr := []rune(msg); len(mr) > 2000 {
		msg = string(mr[:2000])
	}

	// Build status
	status := &core.WorkflowStatus{
		WorkflowName:  metadata.WorkflowName,
		CurrentStep:   currentStepIndex,
		TotalSteps:    metadata.TotalSteps,
		Status:        reqStatus.State,
		CurrentStepID: req.ID,
		StartedAt:     time.Unix(metadata.StartedAt, 0),
		UpdatedAt:     reqStatus.UpdatedAt,
		Message:       msg,
	}

	// Calculate progress using centralized helper
	status.Progress = workflow.CalculateWorkflowStatusProgress(status, reqStatus)
	currentStepProgress := reqStatus.ProgressPercent / 100.0
	w.logger.Debug("Calculated workflow progress",
		zap.String("workflow", status.WorkflowName),
		zap.Int("totalSteps", metadata.TotalSteps),
		zap.Int("currentStepIndex", currentStepIndex),
		zap.Float64("currentStepProgress", currentStepProgress),
		zap.String("state", string(reqStatus.State)),
		zap.Float64("progress", status.Progress))

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
	// Get current request and parse metadata
	_, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to execute step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Delegate execution to RequestService which will lookup the handler
	err = w.requestSvc.ExecuteRequest(ctx, requestID)
	if err != nil {
		// Always call FailWorkflowStep to handle failure according to step's behavior
		return w.FailWorkflowStep(ctx, requestID, err.Error())
	}

	// Execution succeeded - do not automatically complete
	// Let the caller decide when to advance to the next step
	return nil
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

	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return nil, err
	}

	currentStepIndex, err := w.getStepIndexAndStep(wf, metadata.CurrentStepID)
	if err != nil {
		return nil, err
	}

	currentStep := wf.Steps[currentStepIndex]

	return &core.WorkflowStepInfo{
		Operation:       currentStep.Operation,
		FailureBehavior: currentStep.FailureBehavior,
		Status:          req.Status,
	}, nil
}

// dispatchToCron registers a background step for async execution via cron
func (w *WorkflowCoordinatorDefault) dispatchToCron(ctx context.Context, requestID uint) error {
	// Create and register the workflow step executor job
	job, err := w.cronService.JobFactory().CreateJob(workflowStepExecutorJobType)
	if err != nil {
		return fmt.Errorf("failed to create job instance: %w", err)
	}

	job.SetArgs(requestID)

	if err = w.cronService.RegisterJob(job, noRetryPolicy); err != nil {
		return fmt.Errorf("failed to register workflow step job: %w", err)
	}
	return nil
}

func (w *WorkflowCoordinatorDefault) GetWorkflowMetadata(ctx context.Context, requestID uint) (*koanf.Koanf, error) {
	// Get current request and parse metadata
	_, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return nil, err
	}

	k, err := w.jsonToKoanf(string(metadata.Data))
	if err != nil {
		return nil, err
	}

	return k, nil
}

func (w *WorkflowCoordinatorDefault) UpdateWorkflowData(ctx context.Context, requestID uint, data map[string]any) error {
	// Get current request and parse metadata
	req, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
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

	// Get the existing request without trying to parse workflow metadata
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Get the workflow definition
	wf, err := w.GetWorkflow(workflowName)
	if err != nil {
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	if startStep < 0 || startStep >= len(wf.Steps) {
		return core.NewWorkflowError(core.ErrKeyWorkflowStepNotFound, fmt.Errorf("invalid start step: %d", startStep))
	}

	// Create initial metadata
	metadata := WorkflowMetadata{
		WorkflowName:  wf.Name,
		CurrentStepID: wf.Steps[startStep].ID,
		TotalSteps:    len(wf.Steps),
		StartedAt:     time.Now().Unix(),
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
	req.Operation = wf.Steps[startStep].Operation

	w.handleRequestData(processedOpts, req)

	// Save the updated request
	if err := w.requestSvc.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) CleanupWorkflow(ctx context.Context, requestID uint) error {
	// Get the initial request and parse metadata
	_, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
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

func (w *WorkflowCoordinatorDefault) buildWorkflowInstance(ctx context.Context, req *models.Request) (*core.WorkflowInstance, error) {
	status, err := w.GetWorkflowStatus(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for request %d: %w", req.ID, err)
	}

	stepInfo, err := w.GetWorkflowStepInfo(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get step info for request %d: %w", req.ID, err)
	}

	return &core.WorkflowInstance{
		Request:     req,
		Status:      status,
		CurrentStep: stepInfo,
	}, nil
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
		rootID := w.findWorkflowChainRoot(ctx, req.ID)

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

			// Find the step index for this request
			stepIndex := w.getStepIndex(meta.CurrentStepID, workflowName)
			if stepIndex == -1 {
				w.logger.Warn("Unknown step ID in workflow chain",
					zap.String("workflow", workflowName),
					zap.String("stepID", meta.CurrentStepID),
					zap.Uint("requestID", req.ID))
				continue
			}
			if stepIndex == meta.TotalSteps-1 {
				finalReq = req
				break // Found the final request
			}
		}

		if finalReq == nil {
			continue
		}

		instance, err := w.buildWorkflowInstance(ctx, finalReq)
		if err != nil {
			return nil, err
		}

		results = append(results, instance)
	}

	return results, nil
}

func (w *WorkflowCoordinatorDefault) ListWorkflowInstances(ctx context.Context, userID uint, filters []queryutil.CrudFilter, sorts []queryutil.Sort, pagination queryutil.Pagination) ([]*core.WorkflowInstance, int64, error) {
	// Build base query for requests belonging to the user
	baseQuery := w.db.Model(&models.Request{}).Where(&models.Request{UserID: lo.ToPtr(userID)})

	// Add filter to only include requests with workflow metadata
	// Add workflow-specific filters
	workflowFilters := []queryutil.CrudFilter{
		// Only include requests with workflow metadata
		queryutil.FieldIsNotNull("metadata"),
		// Only include requests with workflow_name set
		queryutil.FieldIsNotNull("metadata.workflow_name"),
		// Only include final steps in workflow chains (where next_request_id is 0 or null)
		queryutil.Or(
			queryutil.FieldEqual("metadata.next_request_id", 0),
			queryutil.FieldIsNull("metadata.next_request_id"),
		),
	}

	allFilters := append(filters, workflowFilters...)

	// Apply filters using the queryutil builder
	filteredQuery := queryutil.ApplyFilters(baseQuery, allFilters, nil)

	// Apply sorts
	sortedQuery := queryutil.ApplySort(filteredQuery, sorts)

	// Apply pagination
	paginatedQuery := queryutil.ApplyPagination(sortedQuery, pagination)

	// Execute query to get filtered, sorted, and paginated requests
	var requests []*models.Request
	if err := paginatedQuery.Find(&requests).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query requests: %w", err)
	}

	// Process requests to create workflow instances
	var workflowInstances []*core.WorkflowInstance
	for _, req := range requests {
		var metadata WorkflowMetadata
		if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
			w.logger.Warn("Failed to parse metadata for request",
				zap.Uint("requestID", req.ID),
				zap.Error(err))
			continue
		}

		// Only include requests that actually have workflow metadata
		if metadata.WorkflowName == "" {
			continue
		}

		instance, err := w.buildWorkflowInstance(ctx, req)
		if err != nil {
			w.logger.Warn("Failed to build workflow instance for request",
				zap.Uint("requestID", req.ID),
				zap.Error(err))
			continue
		}

		workflowInstances = append(workflowInstances, instance)
	}

	// Get total count for pagination
	var totalCount int64
	countQuery := queryutil.ApplyFilters(baseQuery, allFilters, nil)
	if err := countQuery.Count(&totalCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	return workflowInstances, totalCount, nil
}

func (w *WorkflowCoordinatorDefault) ListDistinctWorkflowFilters(ctx context.Context, userID uint, additionalFilters []queryutil.CrudFilter) (map[string][]string, error) {
	// Create workflow-specific filters
	workflowFilters := []queryutil.CrudFilter{
		// Only include requests with workflow metadata
		queryutil.FieldIsNotNull("metadata"),
		queryutil.FieldIsNotNull("metadata.workflow_name"),
		// Only include terminal steps in workflow chains (where next_request_id is 0 or null)
		queryutil.Or(
			queryutil.FieldEqual("metadata.next_request_id", 0),
			queryutil.FieldIsNull("metadata.next_request_id"),
		),
	}

	// Combine workflow filters with any additional filters
	allFilters := append(workflowFilters, additionalFilters...)

	// Delegate to request service's ListDistinctRequestFilters with all filters
	return w.requestSvc.ListDistinctRequestFilters(ctx, userID, allFilters)
}

func (w *WorkflowCoordinatorDefault) GetWorkflowInstance(ctx context.Context, userID uint, requestID uint) (*core.WorkflowInstance, error) {
	// First verify the request exists and belongs to the user
	req, err := w.requestSvc.GetRequestWithDeleted(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	if req.UserID == nil || *req.UserID != userID {
		return nil, fmt.Errorf("request does not belong to user")
	}

	// Parse metadata to get workflow name
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return nil, core.NewWorkflowError(core.ErrKeyWorkflowMetadataInvalid, fmt.Errorf("invalid workflow metadata: %w", err))
	}

	return w.buildWorkflowInstance(ctx, req)
}

func (w *WorkflowCoordinatorDefault) scheduleRetry(ctx context.Context, requestID uint) error {
	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Reset status to pending to allow retry
	err = db.RetryableTransaction(w.ctx, w.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", req.ID).
			Updates(map[string]interface{}{
				"status": models.RequestStatusPending,
			})
	})
	if err != nil {
		return err
	}

	// Use DispatchWorkflowStep to retry according to step's configuration
	return w.DispatchWorkflowStep(ctx, requestID)
}

// getStepIndex finds the index of a step by its ID in a workflow
func (w *WorkflowCoordinatorDefault) getStepIndex(stepID string, workflowName string) int {
	wf, err := w.GetWorkflow(workflowName)
	if err != nil {
		return -1
	}

	for i, step := range wf.Steps {
		if step.ID == stepID {
			return i
		}
	}

	return -1
}

// findWorkflowChainRoot follows the PrevRequestID chain backwards to find the root request ID
func (w *WorkflowCoordinatorDefault) findWorkflowChainRoot(ctx context.Context, requestID uint) uint {
	rootID := requestID

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

	return rootID
}

// getStepIndexAndStep finds both the index and step by its ID in a workflow definition
func (w *WorkflowCoordinatorDefault) getStepIndexAndStep(wf *core.WorkflowDefinition, stepID string) (int, error) {
	_, currentStepIndex, found := lo.FindIndexOf(wf.Steps, func(step core.OperationStep) bool {
		return step.ID == stepID
	})

	if !found {
		return -1, core.NewWorkflowError(core.ErrKeyWorkflowStepNotFound, fmt.Errorf("current step ID '%s' not found in workflow", stepID))
	}

	return currentStepIndex, nil
}

// getStepByID finds a step by its ID in a workflow definition
func (w *WorkflowCoordinatorDefault) getStepByID(wf *core.WorkflowDefinition, stepID string) (core.OperationStep, error) {
	currentStepIndex, err := w.getStepIndexAndStep(wf, stepID)
	if err != nil {
		return core.OperationStep{}, err
	}

	return wf.Steps[currentStepIndex], nil
}

// DispatchWorkflowStep runs the current step inline when marked foreground, or schedules it via cron otherwise
func (w *WorkflowCoordinatorDefault) DispatchWorkflowStep(ctx context.Context, requestID uint) error {
	// Get current request and parse metadata
	_, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.logger.Warn("Attempted to dispatch step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Get workflow and current step
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStep, err := w.getStepByID(wf, metadata.CurrentStepID)
	if err != nil {
		return err
	}

	// Execute or dispatch based on step configuration
	if currentStep.Foreground {
		return w.ExecuteWorkflowStep(ctx, requestID)
	}
	return w.dispatchToCron(ctx, requestID)
}

// getCurrentStepIndex gets the current step index from workflow metadata
func (w *WorkflowCoordinatorDefault) getCurrentStepIndex(metadata WorkflowMetadata) (int, error) {
	// Get workflow definition
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return -1, err
	}

	// Get current step index
	_, currentStepIndex, found := lo.FindIndexOf(wf.Steps, func(step core.OperationStep) bool {
		return step.ID == metadata.CurrentStepID
	})

	if !found {
		return -1, core.NewWorkflowError(core.ErrKeyWorkflowStepNotFound, fmt.Errorf("current step ID '%s' not found in workflow", metadata.CurrentStepID))
	}

	return currentStepIndex, nil
}
