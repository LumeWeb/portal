package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	workflowMetrics "go.lumeweb.com/portal/service/internal/workflow"
	workflowPkg "go.lumeweb.com/portal/service/internal/workflow"
	"go.lumeweb.com/queryutil"

	"go.opentelemetry.io/otel/trace"

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

	// ErrUnknownError is used when a nil error is passed to FailWorkflowStep
	ErrUnknownError = errors.New("unknown error")
)

// isPermanentError checks if the error indicates a permanent condition
// that will never succeed on retry (e.g. record not found, resource deleted).
// Retrying such errors creates infinite loops since the underlying resource
// will never appear.
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	// gorm.ErrRecordNotFound covers all "no pin found with request ID" type errors
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	// Check if it's a core Error with a NotFound HTTP status code
	// (e.g. workflow step not found, pin not found)
	if coreErr, ok := err.(*core.Error); ok {
		if coreErr.HttpStatus() == http.StatusNotFound {
			return true
		}
	}
	return false
}

// NewWorkflowError creates a new workflow error
func NewWorkflowError(key core.WorkflowErrorType, err error) *core.Error {
	return core.NewWorkflowError(key, err)
}

// Register service
func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.WORKFLOW_SERVICE,
		Factory: NewWorkflowCoordinator,
		Depends: []string{core.REQUEST_SERVICE, core.CRON_SERVICE},
		Metrics: workflowMetrics.GetCollectors(),
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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.getRequestWithWorkflowMetadata")
	defer span.End()

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

// workflowSpanOpts builds OTEL span options for a workflow step. It creates
// trace links to the root workflow span and the previous step's span (if
// different), plus sets workflow attributes for query-based grouping.
//
// This is the core of the trace linking strategy: each step gets its own
// trace, linked to the root via span links. This works across process/node
// boundaries since all data comes from serialized WorkflowMetadata.
func workflowSpanOpts(req *models.Request, metadata WorkflowMetadata) []core.SpanOption {
	var links []trace.Link
	if metadata.RootTraceParent != "" {
		links = append(links, core.SpanLinksFromTraceParents(metadata.RootTraceParent)...)
	}
	// Link to the previous step's trace too, but only if it's different
	// from the root (first step's TraceParent == RootTraceParent).
	if metadata.TraceParent != "" && metadata.TraceParent != metadata.RootTraceParent {
		links = append(links, core.SpanLinksFromTraceParents(metadata.TraceParent)...)
	}

	opts := []core.SpanOption{
		core.WithLinks(links...),
	}
	if metadata.WorkflowID != "" || metadata.WorkflowName != "" {
		var hashStr string
		if req != nil {
			if sh := core.NewStorageHashFromMultihash(req.Hash, req.CIDType, nil); sh != nil {
				hashStr = sh.CIDString()
			}
		}
		var userID *uint
		if req != nil {
			userID = req.UserID
		}
		opts = append(opts, core.WithAttributes(
			core.WorkflowSpanAttributes(metadata.WorkflowID, metadata.WorkflowName, userID, hashStr)...,
		))
	}
	return opts
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
	WorkflowName  string `json:"workflow_name"`
	CurrentStepID string `json:"current_step_id"`
	TotalSteps    int    `json:"total_steps"`
	NextRequestID uint   `json:"next_request_id,omitempty"`
	PrevRequestID uint   `json:"prev_request_id,omitempty"`
	StartedAt     int64  `json:"started_at"`
	// RetryCount tracks how many times the current step has been retried.
	// Reset to 0 when the step succeeds and the workflow advances.
	RetryCount int `json:"retry_count,omitempty"`
	// TraceParent is the span context of the most recent step. Each step
	// updates it so the next step links to this step's trace.
	TraceParent string `json:"trace_parent,omitempty"`
	// RootTraceParent is the span context of the StartWorkflow call. It
	// never changes across steps. Every step links to this span context
	// to form a trace bundle visible in Tempo's linked traces view.
	RootTraceParent string `json:"root_trace_parent,omitempty"`
	// WorkflowID is the root request ID as a string. Set once at
	// StartWorkflow, used as a span attribute for query-based grouping.
	WorkflowID string         `json:"workflow_id,omitempty"`
	Data       datatypes.JSON `json:"data"`
}

// WorkflowCoordinatorDefault implements the WorkflowCoordinator interface
type WorkflowCoordinatorDefault struct {
	*core.BaseComponent
	requestSvc      core.RequestService
	cronService     core.CronService
	workflows       map[string]*core.WorkflowDefinition
	disabled        map[string]bool
	workflowsMu     sync.RWMutex
	strandedMu      sync.Mutex          // guards strandedWarned
	strandedWarned  map[string]struct{} // dedup keys for warned stranded requests
}

func (w *WorkflowCoordinatorDefault) RegisterTasks(ctx context.Context, cron core.CronService) error {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.RegisterTasks")
	defer span.End()

	err := cron.RegisterJobType(ctx, workflowStepExecutorJobType, func() (core.CronJob, error) {
		return newWorkflowStepExecutorJob(), nil
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) ScheduleJobs(ctx context.Context, cron core.CronService) error {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.ScheduleJobs")
	defer span.End()

	return nil
}

// NewWorkflowCoordinator creates a new workflow coordinator
func NewWorkflowCoordinator() (core.Service, []core.ContextBuilderOption, error) {
	coordinator := &WorkflowCoordinatorDefault{
		workflows:      make(map[string]*core.WorkflowDefinition),
		disabled:       make(map[string]bool),
		strandedWarned: make(map[string]struct{}),
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			coordinator.requestSvc = core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
			coordinator.cronService = core.GetService[core.CronService](ctx, core.CRON_SERVICE)

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

	// Update total workflows gauge
	workflowMetrics.WorkflowsTotal.WithLabelValues().Set(float64(len(w.workflows)))

	w.Logger().Debug("Registered workflow",
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
	w.Logger().Debug("Disabled workflow", zap.String("name", name))
	return nil
}

func (w *WorkflowCoordinatorDefault) EnableWorkflow(name string) error {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()

	if _, exists := w.workflows[name]; !exists {
		return core.NewWorkflowError(core.ErrKeyWorkflowNotFound, fmt.Errorf("workflow '%s' not found", name))
	}

	delete(w.disabled, name)
	w.Logger().Debug("Enabled workflow", zap.String("name", name))
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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.StartWorkflow")
	defer span.End()

	return core.MetricTrackResult(
		workflowMetrics.WorkflowDuration.WithLabelValues(workflowMetrics.LabelOperationStart),
		workflowMetrics.WorkflowsFailed.WithLabelValues(workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelFailureBehaviorFail),
		func() (*models.Request, error) {
			// Get workflow
			wf, err := w.GetWorkflow(name)
			if err != nil {
				return nil, core.NewWorkflowError(core.ErrKeyWorkflowNotFound, fmt.Errorf("workflow '%s' not found: %w", name, err))
			}

			w.workflowsMu.RLock()
			disabled := w.disabled[name]
			w.workflowsMu.RUnlock()

			if disabled {
				w.Logger().Warn("Attempted to start disabled workflow",
					zap.String("workflow", name))
				return nil, nil
			}

			if len(wf.Steps) == 0 {
				return nil, core.ErrWorkflowHasNoSteps
			}

			// First step
			firstStep := wf.Steps[0]

			// Capture trace context for cross-boundary propagation
			traceParent := core.MarshalTraceParent(ctx)

			// Create and process metadata for first step.
			// RootTraceParent is set here so it's persisted in the initial
			// CreateRequest — WorkflowID (which needs createdReq.ID) is
			// updated post-create as non-fatal observability data.
			metadata := WorkflowMetadata{
				WorkflowName:    wf.Name,
				CurrentStepID:   firstStep.ID,
				TotalSteps:      len(wf.Steps),
				StartedAt:       time.Now().Unix(),
				PrevRequestID:   0, // Explicitly initialize
				NextRequestID:   0, // Explicitly initialize
				TraceParent:     traceParent,
				RootTraceParent: traceParent,
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

			// Now that we have the root request ID, set WorkflowID for
			// span attribute-based querying. This is observability-only —
			// failure is non-fatal since the workflow is already created.
			metadata.WorkflowID = strconv.FormatUint(uint64(createdReq.ID), 10)
			metadataJSON, err = json.Marshal(metadata)
			if err != nil {
				w.Logger().Warn("failed to marshal workflow trace metadata", zap.Error(err))
			} else {
				createdReq.Metadata = datatypes.JSON(metadataJSON)
				if err := w.requestSvc.UpdateRequest(ctx, createdReq); err != nil {
					w.Logger().Warn("failed to set workflow trace metadata", zap.Error(err))
				}
			}

			// Record workflow started metric
			protocol := createdReq.Protocol
			if protocol == "" {
				protocol = workflowMetrics.LabelWorkflowUnknown
			}
			workflowMetrics.WorkflowsStarted.WithLabelValues(wf.Name, protocol).Inc()
			workflowMetrics.WorkflowsActive.WithLabelValues(string(models.RequestStatusPending)).Inc()

			w.Logger().Info("Started workflow",
				zap.String("workflow", wf.Name),
				zap.Uint("requestID", createdReq.ID))

			// Auto-trigger first step if configured
			if wf.AutoTriggerFirstStep {
				firstStep = wf.Steps[0]
				if firstStep.Foreground {
					if err := w.ExecuteWorkflowStep(ctx, createdReq.ID); err != nil {
						w.Logger().Error("Failed to execute first step",
							zap.String("workflow", wf.Name),
							zap.Uint("requestID", createdReq.ID),
							zap.Error(err))
						return createdReq, nil // Return request even if execution fails
					}
					// After execution, complete the step to advance to next step
					if err := w.CompleteWorkflowStep(ctx, createdReq.ID); err != nil {
						w.Logger().Error("Failed to complete first step",
							zap.String("workflow", wf.Name),
							zap.Uint("requestID", createdReq.ID),
							zap.Error(err))
						return createdReq, nil
					}
				} else {
					if err := w.DispatchWorkflowStep(ctx, createdReq.ID); err != nil {
						w.Logger().Error("Failed to dispatch first step to cron",
							zap.String("workflow", wf.Name),
							zap.Uint("requestID", createdReq.ID),
							zap.Error(err))
						return createdReq, nil // Return request even if cron dispatch fails
					}
				}
			}

			return createdReq, nil
		},
	)
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
		return metadata, core.NewWorkflowError(core.ErrKeyWorkflowMetadataInvalid, fmt.Errorf("invalid workflow meta: %w", err))
	}
	return metadata, nil
}

func (w *WorkflowCoordinatorDefault) CompleteWorkflowStep(ctx context.Context, requestID uint, opts ...core.WorkflowOption) error {
	// Fetch request and metadata once — used for span links/attributes
	// and reused inside the MetricTrack closure to avoid a second DB round trip.
	req, metadata, traceErr := w.getRequestWithWorkflowMetadata(ctx, requestID)

	// Create span with links to root + previous step traces, plus workflow
	// attributes for query-based grouping. Each step is its own trace.
	var spanOpts []core.SpanOption
	if traceErr == nil {
		spanOpts = workflowSpanOpts(req, metadata)
	}
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.CompleteWorkflowStep",
		spanOpts...,
	)
	defer span.End()

	return core.MetricTrack(
		workflowMetrics.WorkflowDuration.WithLabelValues(workflowMetrics.LabelOperationComplete),
		workflowMetrics.WorkflowsFailed.WithLabelValues(workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelFailureBehaviorFail),
		func() error {
			if traceErr != nil {
				// Outer fetch failed (possibly transient) before MetricTrack
				// timing began; retry once so a transient DB error does not
				// permanently fail this step.
				req, metadata, traceErr = w.getRequestWithWorkflowMetadata(ctx, requestID)
				if traceErr != nil {
					return traceErr
				}
			}

			nextMetadata := metadata

			if w.isDisabled(metadata.WorkflowName) {
				w.Logger().Warn("Attempted to complete step in disabled workflow",
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

			// Get current step for metrics
			currentStep, err := w.getStepByID(wf, metadata.CurrentStepID)
			if err != nil {
				return err
			}

			// Record step completed metric
			workflowMetrics.WorkflowStepsCompleted.WithLabelValues(wf.Name, currentStep.Operation).Inc()

			// Check if workflow is complete
			if currentStepIndex >= len(wf.Steps)-1 {
				w.Logger().Info("Workflow completed",
					zap.String("workflow", metadata.WorkflowName),
					zap.Uint("requestID", requestID))

				// Record workflow completed metric
				protocol := req.Protocol
				if protocol == "" {
					protocol = workflowMetrics.LabelWorkflowUnknown
				}
				workflowMetrics.WorkflowsCompleted.WithLabelValues(wf.Name, protocol).Inc()

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
			// Reset retry count for the next step
			nextMetadata.RetryCount = 0
			// Update trace parent so the next step's spans link to the
			// same trace, even when dispatched via cron or across nodes.
			nextMetadata.TraceParent = core.MarshalTraceParent(ctx)
			// Validate and convert JSON data
			if len(wfMetadataJSON) == 0 {
				nextMetadata.Data = datatypes.JSON("{}") // Default to empty JSON object
			} else if !json.Valid(wfMetadataJSON) {
				return core.NewWorkflowError(core.ErrKeyWorkflowMetadataInvalid, fmt.Errorf("invalid JSON data for workflow metadata: %s", string(wfMetadataJSON)))
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
		},
	)
}

// FailWorkflowStep handles a step failure
func (w *WorkflowCoordinatorDefault) FailWorkflowStep(ctx context.Context, requestID uint, stepErr error) error {
	// Fetch request+metadata for span links/attributes. Use defensive
	// error handling since the caller may already be in an error path.
	currentReq, traceMetadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	var spanOpts []core.SpanOption
	if err == nil {
		spanOpts = workflowSpanOpts(currentReq, traceMetadata)
	}

	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.FailWorkflowStep",
		spanOpts...,
	)
	defer span.End()

	// Defensively handle nil error parameter
	if stepErr == nil {
		stepErr = ErrUnknownError
	}

	// Store the original error for later use
	originalErr := stepErr

	// Get current request and parse metadata
	if err != nil {
		return err
	}
	metadata := traceMetadata

	if w.isDisabled(metadata.WorkflowName) {
		w.Logger().Warn("Attempted to fail step in disabled workflow",
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
	if failErr := w.requestSvc.FailRequest(ctx, currentReq.ID, originalErr.Error()); failErr != nil {
		return failErr
	}

	// Handle according to failure behavior
	switch currentStep.FailureBehavior {
	case core.FailWorkflow:
		w.Logger().Info("Workflow failed",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID),
			zap.Error(originalErr))
		return nil

	case core.ContinueWorkflow:
		// Continue to next step despite failure
		return w.CompleteWorkflowStep(ctx, requestID)

	case core.RetryStep:
		// Check if the failure is a quota error - if so, do not retry
		if core.IsQuotaExceededError(originalErr) {
			w.Logger().Info("Skipping retry due to quota exceeded error",
				zap.String("workflow", metadata.WorkflowName),
				zap.Uint("requestID", requestID),
				zap.Error(originalErr))
			return nil
		}

		// Check if the failure is a permanent/not-found error - if so, do not retry
		if isPermanentError(originalErr) {
			w.Logger().Info("Skipping retry due to permanent (not-found) error",
				zap.String("workflow", metadata.WorkflowName),
				zap.Uint("requestID", requestID),
				zap.Error(originalErr))
			return nil
		}

		// Record step retried metric
		workflowMetrics.WorkflowStepsRetried.WithLabelValues(wf.Name, currentStep.Operation).Inc()

		// Schedule retry with backoff
		retryErr := w.scheduleRetry(ctx, requestID)
		if retryErr != nil {
			return retryErr
		}
		// Return a retried error to indicate this was a retry
		return core.NewWorkflowError(core.ErrKeyWorkflowStepRetried, nil)
	}

	return nil
}

// GetWorkflowStatus returns the current status of a workflow
func (w *WorkflowCoordinatorDefault) GetWorkflowStatus(ctx context.Context, requestID uint) (*core.WorkflowStatus, error) {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.GetWorkflowStatus")
	defer span.End()

	return core.MetricTrackResult(
		workflowMetrics.WorkflowDuration.WithLabelValues(workflowMetrics.LabelOperationGetStatus),
		workflowMetrics.WorkflowsFailed.WithLabelValues(workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelWorkflowUnknown, workflowMetrics.LabelFailureBehaviorFail),
		func() (*core.WorkflowStatus, error) {
			// Follow NextRequestID chain to find the latest step
			latestID := w.findLatestRequest(ctx, requestID)

			// Get request for the latest step
			req, err := w.requestSvc.GetRequestWithDeleted(ctx, latestID)
			if err != nil {
				return nil, err
			}

			// Parse metadata
			metadata, err := w.parseWorkflowMetadata(req.Metadata)
			if err != nil {
				return nil, err
			}

			// Get detailed request status for the latest step
			reqStatus, err := w.requestSvc.GetRequestStatus(ctx, latestID, true)
			if err != nil {
				return nil, err
			}

			currentStepIndex, effectiveTotalSteps, err := w.getCurrentStepIndex(metadata, req.ID)
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
				TotalSteps:    effectiveTotalSteps,
				Status:        reqStatus.State,
				CurrentStepID: req.ID,
				StartedAt:     time.Unix(metadata.StartedAt, 0),
				UpdatedAt:     reqStatus.UpdatedAt,
				Message:       msg,
			}

			// Calculate progress using centralized helper
			status.Progress = workflowPkg.CalculateWorkflowStatusProgress(status, reqStatus)
			// Normalize to 2 decimal places to ensure consistent JSON output for UI clients
			status.Progress = math.Round(status.Progress*100) / 100
			currentStepProgress := reqStatus.ProgressPercent / 100.0
			w.Logger().Debug("Calculated workflow progress",
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
					w.Logger().Warn("Previous request not found in workflow chain",
						zap.Uint("requestID", prevID),
						zap.Error(err))
					break
				}

				var prevMetadata WorkflowMetadata
				if err := json.Unmarshal(prevReq.Metadata, &prevMetadata); err != nil {
					w.Logger().Warn("Failed to parse metadata for previous request",
						zap.Uint("requestID", prevID),
						zap.Error(err))
					break
				}

				prevID = prevMetadata.PrevRequestID
			}

			status.PreviousSteps = previousSteps

			return status, nil
		},
	)
}

// ExecuteWorkflowStep executes the operation handler for a workflow step
func (w *WorkflowCoordinatorDefault) ExecuteWorkflowStep(ctx context.Context, requestID uint) error {
	// Get current request and parse metadata first to get workflow info for metrics
	req, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
	}

	// Create span with links to root + previous step traces, plus workflow
	// attributes for query-based grouping. Each step is its own trace, linked
	// to the workflow root — the OTEL pattern for fan-out/async workflows.
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.ExecuteWorkflowStep",
		workflowSpanOpts(req, metadata)...,
	)
	defer span.End()

	if w.isDisabled(metadata.WorkflowName) {
		w.Logger().Warn("Attempted to execute step in disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Get workflow and current step to check failure behavior
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStep, err := w.getStepByID(wf, metadata.CurrentStepID)
	if err != nil {
		return err
	}

	// Determine execution type for metrics
	executionType := workflowMetrics.LabelStepExecutionBackground
	if currentStep.Foreground {
		executionType = workflowMetrics.LabelStepExecutionForeground
	}

	return core.MetricTrack(
		workflowMetrics.WorkflowStepDuration.WithLabelValues(wf.Name, currentStep.Operation),
		workflowMetrics.WorkflowStepsFailed.WithLabelValues(wf.Name, currentStep.Operation, workflowMetrics.LabelFailureBehaviorFail),
		func() error {
			// Delegate execution to RequestService which will lookup the handler
			err = w.requestSvc.ExecuteRequest(ctx, requestID)
			if err != nil {
				// Record step failed metric
				failureBehavior := workflowMetrics.LabelFailureBehaviorFail
				switch currentStep.FailureBehavior {
				case core.ContinueWorkflow:
					failureBehavior = workflowMetrics.LabelFailureBehaviorContinue
				case core.RetryStep:
					failureBehavior = workflowMetrics.LabelFailureBehaviorRetry
				}
				workflowMetrics.WorkflowStepsFailed.WithLabelValues(wf.Name, currentStep.Operation, failureBehavior).Inc()

				// Always call FailWorkflowStep to handle failure according to step's behavior
				failErr := w.FailWorkflowStep(ctx, requestID, err)

				// If the step is configured to continue on failure, return the original error
				// to indicate that execution failed, but the workflow should continue.
				// The caller will decide whether to call CompleteWorkflowStep based on the
				// failure behavior.
				if currentStep.FailureBehavior == core.ContinueWorkflow {
					return err
				}

				return failErr
			}

			// Record step executed metric
			workflowMetrics.WorkflowStepsExecuted.WithLabelValues(wf.Name, currentStep.Operation, executionType).Inc()

			// Execution succeeded - do not automatically complete
			// Let the caller decide when to advance to the next step
			return nil
		},
	)
}

// CanTransition checks if a workflow step can be transitioned from its current state
func (w *WorkflowCoordinatorDefault) CanTransition(ctx context.Context, requestID uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.CanTransition")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.GetWorkflowStepInfo")
	defer span.End()

	// Follow NextRequestID chain to find the latest step
	latestID := w.findLatestRequest(ctx, requestID)

	// Get current request
	req, err := w.requestSvc.GetRequestWithDeleted(ctx, latestID)
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
		// The step ID no longer exists in the workflow definition, likely
		// because the workflow was refactored. Return a synthetic step info
		// using the request's own data so listing/status queries don't break.
		if core.IsWorkflowErrorType(err, core.ErrKeyWorkflowStepNotFound) {
			w.warnStrandedStep(metadata.WorkflowName, metadata.CurrentStepID, req.ID)
			return &core.WorkflowStepInfo{
				Operation: req.Operation,
				// FailureBehavior intentionally left at its zero value
				// (FailWorkflow) when the removed step's original behavior
				// cannot be recovered from request metadata, rather than
				// fabricating RetryStep semantics.
				Status: req.Status,
			}, nil
		}
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
	return w.dispatchToCronWithDelay(ctx, requestID, 0)
}

// dispatchToCronWithDelay registers a background step for async execution via cron
// with the specified delay before the first execution.
func (w *WorkflowCoordinatorDefault) dispatchToCronWithDelay(ctx context.Context, requestID uint, delay time.Duration) error {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.dispatchToCronWithDelay")
	defer span.End()

	// Create and register the workflow step executor job
	job, err := w.cronService.JobFactory().CreateJob(ctx, workflowStepExecutorJobType)
	if err != nil {
		return fmt.Errorf("failed to create job instance: %w", err)
	}

	job.SetArgs(requestID)

	// Override the schedule definition with a delayed start time if specified
	if delay > 0 {
		job.SetScheduledDefinition(
			core.NewCronScheduleDefinition(core.CronScheduleTypeOnce).
				WithAtTime(time.Now().Add(delay)),
		)
	}

	if err = w.cronService.RegisterJob(ctx, job, noRetryPolicy); err != nil {
		return fmt.Errorf("failed to register workflow step job: %w", err)
	}
	return nil
}

func (w *WorkflowCoordinatorDefault) GetWorkflowMetadata(ctx context.Context, requestID uint) (*koanf.Koanf, error) {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.GetWorkflowMetadata")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.UpdateWorkflowData")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.UpdateWorkflowDataStruct")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.ConvertRequestToWorkflow")
	defer span.End()

	if w.isDisabled(workflowName) {
		w.Logger().Warn("Attempted to convert request to disabled workflow",
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
		return core.NewWorkflowError(core.ErrKeyWorkflowStepNotFound, fmt.Errorf("invalid start step %d for workflow %s", startStep, workflowName))
	}

	// Create initial metadata
	traceParent := core.MarshalTraceParent(ctx)
	metadata := WorkflowMetadata{
		WorkflowName:    wf.Name,
		CurrentStepID:   wf.Steps[startStep].ID,
		TotalSteps:      len(wf.Steps),
		StartedAt:       time.Now().Unix(),
		TraceParent:     traceParent,
		RootTraceParent: traceParent,
		WorkflowID:      strconv.FormatUint(uint64(requestID), 10),
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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.CleanupWorkflow")
	defer span.End()

	// Get the initial request and parse metadata
	req, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
	}

	if w.isDisabled(metadata.WorkflowName) {
		w.Logger().Warn("Attempted to cleanup disabled workflow",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID))
		return nil
	}

	// Update active workflows gauge based on final status
	if req.Status == models.RequestStatusCompleted {
		workflowMetrics.WorkflowsActive.WithLabelValues(string(models.RequestStatusProcessing)).Dec()
		workflowMetrics.WorkflowsActive.WithLabelValues(string(models.RequestStatusCompleted)).Inc()
	} else if req.Status == models.RequestStatusFailed {
		workflowMetrics.WorkflowsActive.WithLabelValues(string(models.RequestStatusProcessing)).Dec()
		workflowMetrics.WorkflowsActive.WithLabelValues(string(models.RequestStatusFailed)).Inc()

		// Record workflow failed metric for final failure
		protocol := req.Protocol
		if protocol == "" {
			protocol = workflowMetrics.LabelWorkflowUnknown
		}
		workflowMetrics.WorkflowsFailed.WithLabelValues(metadata.WorkflowName, protocol, workflowMetrics.LabelFailureBehaviorFail).Inc()
	}

	return nil
}

func (w *WorkflowCoordinatorDefault) buildWorkflowInstance(ctx context.Context, req *models.Request) (*core.WorkflowInstance, error) {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.buildWorkflowInstance")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.FindWorkflowInstances")
	defer span.End()

	if w.isDisabled(workflowName) {
		return nil, nil
	}

	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	// First find all requests belonging to this workflow
	var allRequests []*models.Request
	if err := db.RetryableComponentTransaction(w, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
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
				w.Logger().Warn("Unknown step ID in workflow chain",
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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.ListWorkflowInstances")
	defer span.End()

	// Build base query for requests belonging to the user
	baseQuery := w.DB().WithContext(ctx).Model(&models.Request{}).Where(&models.Request{UserID: lo.ToPtr(userID)})

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

	// NOTE: pagination is intentionally NOT applied at the SQL level here.
	// Rows are post-filtered in Go (a row whose metadata.workflow_name names a
	// workflow that isn't registered in this coordinator is dropped by
	// buildWorkflowInstance), so SQL OFFSET/LIMIT would page over rows that may
	// not survive filtering, yielding inconsistent items vs total. A correct
	// total for the post-filtered set requires examining every candidate row
	// (each triggers the per-row buildWorkflowInstance work regardless), so we
	// fetch the full sorted candidate set — lightweight Request rows bounded by
	// a single user's workflow history — build the valid instances, then page
	// over the validated slice and report total as the count of valid ones.

	// Execute query to get filtered and sorted requests.
	// The context is inherited from the base query's WithContext, and asserted
	// explicitly here so the query honors request cancellation/timeouts.
	var requests []*models.Request
	if err := sortedQuery.WithContext(ctx).Find(&requests).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query requests: %w", err)
	}

	// Process requests to create workflow instances
	workflowInstances := make([]*core.WorkflowInstance, 0, len(requests))
	for _, req := range requests {
		var metadata WorkflowMetadata
		if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
			w.Logger().Warn("Failed to parse metadata for request",
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
			w.Logger().Warn("Failed to build workflow instance for request",
				zap.Uint("requestID", req.ID),
				zap.Error(err))
			continue
		}

		workflowInstances = append(workflowInstances, instance)
	}

	// total reflects the validated set, so items and total can never disagree.
	totalCount := int64(len(workflowInstances))

	// Apply pagination to the validated slice (Start inclusive, End exclusive,
	// matching the queryutil _start/_end convention).
	offset := pagination.Start
	if offset < 0 {
		offset = 0
	}
	limit := pagination.End - pagination.Start
	if limit <= 0 {
		limit = len(workflowInstances)
	}
	start := offset
	if start > len(workflowInstances) {
		start = len(workflowInstances)
	}
	end := offset + limit
	if end > len(workflowInstances) {
		end = len(workflowInstances)
	}
	if end < start {
		end = start
	}

	return workflowInstances[start:end], totalCount, nil
}

func (w *WorkflowCoordinatorDefault) ListDistinctWorkflowFilters(ctx context.Context, userID uint, additionalFilters []queryutil.CrudFilter) (map[string][]string, error) {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.ListDistinctWorkflowFilters")
	defer span.End()

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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.GetWorkflowInstance")
	defer span.End()

	// Follow NextRequestID chain to find the latest step
	latestID := w.findLatestRequest(ctx, requestID)

	// First verify the request exists and belongs to the user
	req, err := w.requestSvc.GetRequestWithDeleted(ctx, latestID)
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
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.scheduleRetry")
	defer span.End()

	// Get workflow retry config
	wfConfig := w.Config().Config().Core.Cron.Workflow
	maxRetries := wfConfig.MaxRetries
	initialDelay := wfConfig.InitialRetryDelay
	backoffFactor := wfConfig.RetryBackoffFactor

	// Get current request
	req, err := w.requestSvc.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}

	// Parse current metadata
	var metadata WorkflowMetadata
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return fmt.Errorf("failed to parse workflow metadata: %w", err)
	}

	// Increment retry count
	metadata.RetryCount++

	// Check against max retries. Use >= so max_retries=0 fails after the
	// first attempt (RetryCount=1), preventing unbounded inline recursion
	// for foreground steps whose operation permanently fails.
	if metadata.RetryCount >= maxRetries {
		w.Logger().Error("Workflow step exceeded max retries - failing permanently",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID),
			zap.Int("retryCount", metadata.RetryCount),
			zap.Int("maxRetries", maxRetries))

		// Persist the final RetryCount in metadata
		metadataJSON, jsonErr := json.Marshal(metadata)
		if jsonErr == nil {
			_ = db.RetryableComponentTransaction(w, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&models.Request{}).
					Where("id = ?", req.ID).
					Update("metadata", metadataJSON)
			})
		}

		// The request was already marked as Failed by FailWorkflowStep.
		// Update the status message to reflect the permanent failure.
		failMsg := fmt.Sprintf("step exceeded maximum retries (%d)", maxRetries)
		_ = w.requestSvc.FailRequest(ctx, req.ID, failMsg)

		// Return a non-nil error so FailWorkflowStep does not wrap
		// the return in ErrKeyWorkflowStepRetried.
		return fmt.Errorf("step exceeded maximum retries (%d)", maxRetries)
	}

	// Calculate backoff delay for this retry attempt
	delay := initialDelay * time.Duration(math.Pow(backoffFactor, float64(metadata.RetryCount-1)))

	// Marshal updated metadata with incremented retry count
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow metadata: %w", err)
	}

	// Reset status to pending and persist updated retry count
	err = db.RetryableComponentTransaction(w, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", req.ID).
			Updates(map[string]interface{}{
				"status":         models.RequestStatusPending,
				"status_message": nil,
				"metadata":       metadataJSON,
			})
	})
	if err != nil {
		return err
	}

	// Determine whether to re-execute inline (foreground) or dispatch to
	// cron with backoff (background). Foreground steps re-execute inline to
	// preserve existing behavior; background steps get the calculated delay.
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return err
	}

	currentStep, err := w.getStepByID(wf, metadata.CurrentStepID)
	if err != nil {
		return err
	}

	if currentStep.Foreground {
		// Foreground: re-execute inline (no delay) via DispatchWorkflowStep
		w.Logger().Info("Retrying workflow step (foreground)",
			zap.String("workflow", metadata.WorkflowName),
			zap.Uint("requestID", requestID),
			zap.Int("retryCount", metadata.RetryCount),
			zap.Int("maxRetries", maxRetries))
		return w.DispatchWorkflowStep(ctx, requestID)
	}

	// Background: dispatch to cron with calculated backoff delay
	w.Logger().Info("Scheduling workflow step retry with backoff",
		zap.String("workflow", metadata.WorkflowName),
		zap.Uint("requestID", requestID),
		zap.Int("retryCount", metadata.RetryCount),
		zap.Int("maxRetries", maxRetries),
		zap.Duration("delay", delay))
	return w.dispatchToCronWithDelay(ctx, requestID, delay)
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

// findLatestRequest follows the NextRequestID chain forward to find the latest (current) request ID
func (w *WorkflowCoordinatorDefault) findLatestRequest(ctx context.Context, requestID uint) uint {
	latestID := requestID

	for {
		req, err := w.requestSvc.GetRequestWithDeleted(ctx, latestID)
		if err != nil || req == nil {
			break
		}

		var meta WorkflowMetadata
		if err := json.Unmarshal(req.Metadata, &meta); err != nil {
			break
		}

		if meta.NextRequestID == 0 {
			break
		}

		latestID = meta.NextRequestID
	}

	return latestID
}

// findWorkflowChainRoot follows the PrevRequestID chain backwards to find the root request ID
func (w *WorkflowCoordinatorDefault) findWorkflowChainRoot(ctx context.Context, requestID uint) uint {
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.findWorkflowChainRoot")
	defer span.End()

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
		return -1, core.NewWorkflowError(core.ErrKeyWorkflowStepNotFound, fmt.Errorf("current step ID '%s' not found in workflow '%s'", stepID, wf.Name))
	}

	return currentStepIndex, nil
}

// warnStrandedStep emits a WARN log the first time a given (workflow, stepID, requestID)
// combination is encountered. Subsequent calls for the same key are silently skipped,
// preventing log flooding from repeated status polls on stranded requests.
func (w *WorkflowCoordinatorDefault) warnStrandedStep(workflowName, stepID string, requestID uint) {
	w.strandedMu.Lock()
	defer w.strandedMu.Unlock()

	if len(w.strandedWarned) > 10000 {
		w.strandedWarned = make(map[string]struct{})
	}

	key := fmt.Sprintf("%s/%s/%d", workflowName, stepID, requestID)
	if _, alreadyWarned := w.strandedWarned[key]; alreadyWarned {
		return
	}
	w.strandedWarned[key] = struct{}{}
	w.Logger().Warn("Step ID not found in workflow definition, treating as completed",
		zap.String("workflow", workflowName),
		zap.String("stepID", stepID),
		zap.Uint("requestID", requestID))
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
	req, metadata, err := w.getRequestWithWorkflowMetadata(ctx, requestID)
	if err != nil {
		return err
	}

	// Create span with links to root + previous step traces, plus workflow
	// attributes for query-based grouping. Each step is its own trace.
	ctx, span := core.TraceMethod(ctx, "WorkflowCoordinatorDefault.DispatchWorkflowStep",
		workflowSpanOpts(req, metadata)...,
	)
	defer span.End()

	if w.isDisabled(metadata.WorkflowName) {
		w.Logger().Warn("Attempted to dispatch step in disabled workflow",
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
func (w *WorkflowCoordinatorDefault) getCurrentStepIndex(metadata WorkflowMetadata, requestID uint) (int, int, error) {
	// Get workflow definition
	wf, err := w.GetWorkflow(metadata.WorkflowName)
	if err != nil {
		return -1, 0, err
	}

	// Get current step index
	_, currentStepIndex, found := lo.FindIndexOf(wf.Steps, func(step core.OperationStep) bool {
		return step.ID == metadata.CurrentStepID
	})

	if !found {
		// The step ID no longer exists in the workflow definition, likely
		// because the workflow was refactored and this step was removed or
		// moved to a separate workflow. Treat the request as completed
		// (index = TotalSteps) so status/listing queries don't error and
		// progress renders as 100%. Use metadata.TotalSteps (not len(wf.Steps))
		// because the workflow may have been refactored with a different
		// number of steps, and TotalSteps is what the caller uses for
		// status.TotalSteps and progress calculation. Fall back to len(wf.Steps)
		// if TotalSteps was never populated (e.g. older metadata).
		w.warnStrandedStep(metadata.WorkflowName, metadata.CurrentStepID, requestID)
		total := metadata.TotalSteps
		if total <= 0 {
			total = len(wf.Steps)
		}
		if total < 1 {
			// Workflow was refactored so all steps moved out (len==0) and
			// metadata.TotalSteps was never populated. Use 1 as a floor so
			// CurrentStep (total-1) is never negative.
			total = 1
		}
		// Return total-1 as the step index (0-based, last step) and total as
		// the effective total steps. This keeps CurrentStep within [0, TotalSteps-1]
		// for consumers that treat it as a 0-based index, and lets
		// CalculateProgress render the request's real status instead of masking
		// it as 100% completed.
		return total - 1, total, nil
	}

	return currentStepIndex, metadata.TotalSteps, nil
}

// EmptyStepsForTesting sets a workflow's Steps to an empty slice. This is
// only for testing the defensive guard in getCurrentStepIndex when a
// workflow has been fully refactored (all steps removed). It bypasses
// RegisterWorkflow's validation which rejects 0-step workflows.
func (w *WorkflowCoordinatorDefault) EmptyStepsForTesting(name string) {
	w.workflowsMu.Lock()
	defer w.workflowsMu.Unlock()
	if wf, ok := w.workflows[name]; ok {
		wf.Steps = []core.OperationStep{}
	}
}
