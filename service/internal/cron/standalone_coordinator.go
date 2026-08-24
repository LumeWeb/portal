package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StackFrame represents a single frame in a call stack
type StackFrame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// PanicInfo represents structured information about a panic
type PanicInfo struct {
	Message    string       `json:"message"`
	RecoverVal any          `json:"recoverVal"`
	Stack      []StackFrame `json:"stack,omitempty"`
}

var _ core.CronCoordinator = (*StandaloneCoordinator)(nil)

type StandaloneCoordinator struct {
	ctx           core.Context
	db            *gorm.DB
	jobCreator    *JobCreator
	logger        *core.Logger
	cronService   core.CronService
	stateMachine  core.CronJobStateMachine
	scheduler     gocron.Scheduler
	jobContexts   map[uuid.UUID]context.Context
	jobCancels    map[uuid.UUID]context.CancelFunc
	jobCtxMu      sync.RWMutex
	failureCounts map[uuid.UUID]int
	failureMu     sync.Mutex
	maxFailures   int
}

const (
	maxRetryDelay = 24 * time.Hour // Maximum allowed delay for retries
)

type CoordinatorOptions struct {
	JobCreator   *JobCreator
	Scheduler    gocron.Scheduler
	StateMachine core.CronJobStateMachine
}

func NewCoordinatorOptions() *CoordinatorOptions {
	return &CoordinatorOptions{}
}

func (o *CoordinatorOptions) WithJobCreator(creator *JobCreator) *CoordinatorOptions {
	o.JobCreator = creator
	return o
}

func (o *CoordinatorOptions) WithScheduler(scheduler gocron.Scheduler) *CoordinatorOptions {
	o.Scheduler = scheduler
	return o
}

func (o *CoordinatorOptions) WithStateMachine(stateMachine core.CronJobStateMachine) *CoordinatorOptions {
	o.StateMachine = stateMachine
	return o
}

func NewStandaloneCoordinator(
	ctx core.Context,
	cronService core.CronService,
	registry core.CronJobStateMachineRegistry,
	opts ...*CoordinatorOptions,
) (*StandaloneCoordinator, error) {
	var opt *CoordinatorOptions
	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	} else {
		opt = NewCoordinatorOptions()
	}

	coordinator := &StandaloneCoordinator{
		ctx:           ctx,
		db:            ctx.DB(),
		logger:        ctx.Logger(),
		cronService:   cronService,
		jobContexts:   make(map[uuid.UUID]context.Context),
		jobCancels:    make(map[uuid.UUID]context.CancelFunc),
		failureCounts: make(map[uuid.UUID]int),
		maxFailures:   5, // Allow up to 5 consecutive failures
	}

	if opt.StateMachine != nil {
		coordinator.stateMachine = opt.StateMachine
	} else {
		coordinator.stateMachine = NewCronJobStateMachine(ctx, registry)
	}

	// Set scheduler from options or create default
	if opt.Scheduler != nil {
		coordinator.scheduler = opt.Scheduler
	} else {
		scheduler, err := gocron.NewScheduler(gocron.WithSchedulerMonitor(NewPrometheusMonitor()))
		if err != nil {
			return nil, fmt.Errorf("failed to create scheduler: %w", err)
		}
		coordinator.scheduler = scheduler
	}

	// Set job creator from options or create default
	if opt.JobCreator != nil {
		coordinator.jobCreator = opt.JobCreator
	} else {
		coordinator.jobCreator = NewJobCreator(ctx.DB(), cronService.JobFactory(), ctx.Logger())
	}

	return coordinator, nil
}

func (s *StandaloneCoordinator) SetHeartbeat(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.SetHeartbeat")
	defer span.End()

	// Heartbeat is not a state transition — it's a timestamp update while remaining
	// in Running state. Going through the FSM would require Running→Running which
	// is invalid, and would bump the version on every 30s tick. Direct DB update
	// of last_heartbeat only.
	now := time.Now()
	return db.RetryableTransaction(ctx, s.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.CronJob{}).
			Where("uuid = ?", types.FromUUID(jobID)).
			Update("last_heartbeat", now)
	})
}

func (s *StandaloneCoordinator) CheckHeartbeat(ctx context.Context, jobID uuid.UUID) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.CheckHeartbeat")
	defer span.End()

	var job models.CronJob
	err := db.RetryableTransaction(ctx, s.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("uuid = ?", types.FromUUID(jobID)).First(&job)
	})
	if err != nil {
		return false, err
	}
	if job.LastHeartbeat == nil {
		return false, nil
	}
	return time.Since(*job.LastHeartbeat) < 2*time.Minute, nil
}

func (s *StandaloneCoordinator) CreateJobFromDB(ctx context.Context, jobID uuid.UUID) (core.CronJob, error) {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.CreateJobFromDB")
	defer span.End()

	return s.jobCreator.CreateFromDB(ctx, jobID)
}

func (s *StandaloneCoordinator) getJobRecord(ctx context.Context, jobID uuid.UUID) (*models.CronJob, error) {
	var dbJob models.CronJob
	err := db.RetryableTransaction(ctx, s.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Where(&models.CronJob{UUID: types.FromUUID(jobID)}).First(&dbJob)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get job from DB: %w", err)
	}
	return &dbJob, nil
}

func (s *StandaloneCoordinator) getScheduleDefinition(dbJob *models.CronJob) (*core.CronScheduleDefinition, error) {
	var schedDef core.CronScheduleDefinition
	if len(dbJob.SchedDef) > 0 {
		if err := json.Unmarshal([]byte(dbJob.SchedDef), &schedDef); err != nil {
			return nil, fmt.Errorf("failed to deserialize schedule definition: %w", err)
		}
	}
	return &schedDef, nil
}

func (s *StandaloneCoordinator) getScheduleDefinitionForJob(ctx context.Context, jobID uuid.UUID) (gocron.JobDefinition, error) {
	dbJob, err := s.getJobRecord(ctx, jobID)
	if err != nil {
		return nil, err
	}

	schedDef, err := s.getScheduleDefinition(dbJob)
	if err != nil {
		return nil, err
	}

	return s.cronService.ScheduleRegistry().Create(*schedDef)
}

func (s *StandaloneCoordinator) validateRetryPolicy(ctx context.Context, jobID uuid.UUID) error {
	dbJob, err := s.getJobRecord(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job record for retry policy validation: %w", err)
	}

	if len(dbJob.RetryPolicy) == 0 {
		return nil
	}

	var retryPolicy core.RetryPolicy
	if err := json.Unmarshal([]byte(dbJob.RetryPolicy), &retryPolicy); err != nil {
		return fmt.Errorf("failed to unmarshal retry policy: %w", err)
	}

	if retryPolicy.MaxRetries < 0 {
		s.logger.Error("Invalid retry policy: MaxRetries cannot be negative",
			zap.String("jobID", jobID.String()),
			zap.Int("maxRetries", retryPolicy.MaxRetries))
		return fmt.Errorf("invalid retry policy: MaxRetries cannot be negative")
	}

	if retryPolicy.MaxRetries == 0 {
		return nil
	}

	if retryPolicy.InitialDelay < 0 {
		s.logger.Error("Invalid retry policy: InitialDelay cannot be negative",
			zap.String("jobID", jobID.String()),
			zap.Duration("initialDelay", retryPolicy.InitialDelay))
		return fmt.Errorf("invalid retry policy: InitialDelay cannot be negative")
	}

	if retryPolicy.BackoffFactor < 1 {
		s.logger.Error("Invalid retry policy: BackoffFactor must be >= 1",
			zap.String("jobID", jobID.String()),
			zap.Float64("backoffFactor", retryPolicy.BackoffFactor))
		return fmt.Errorf("invalid retry policy: BackoffFactor must be >= 1")
	}

	return nil
}

func (s *StandaloneCoordinator) EnqueueJob(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.EnqueueJob")
	defer span.End()

	// Validate jobID is not empty
	if jobID == uuid.Nil {
		return fmt.Errorf("invalid job ID: cannot be empty")
	}

	// Ensure we have a valid context before scheduling
	if _, err := s.getOrCreateJobContext(ctx, jobID); err != nil {
		return fmt.Errorf("failed to create job context: %w", err)
	}

	jobDef, err := s.getScheduleDefinitionForJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get schedule definition: %w", err)
	}

	// Define the task function
	// Cron execution is a genuine asynchronous boundary. Start from a fresh,
	// bounded context instead of the incoming request/workflow context graph.
	// Detaching with context.WithoutCancel would carry the entire arbitrary
	// context chain (and, on retries, grow it by one layer each attempt).
	//
	// ExecuteJob creates its own new root span (WithNewRoot), but SetupJob,
	// CleanupJob and HandleFailedJob do not, so carry only the OTel span
	// context (via W3C traceparent) to keep their traces parented while
	// dropping every other value. context.Background() is never canceled,
	// preserving the original intent to outlive the scheduling context.
	jobCtx := context.Background()
	jobCtx = core.WithTracerInfo(jobCtx, core.DefaultTracerService, "cron")
	jobCtx = core.ContextWithTraceParent(jobCtx, core.MarshalTraceParent(ctx))
	taskFunc := func(jobID uuid.UUID) error {
		return s.ExecuteJob(jobCtx, jobID)
	}

	// Validate retry policy before computing delay
	if err := s.validateRetryPolicy(ctx, jobID); err != nil {
		return fmt.Errorf("retry policy validation failed: %w", err)
	}

	// Calculate delay if this is a retry
	var delay time.Duration
	if failures, exists := s.failureCounts[jobID]; exists && failures > 0 {
		var dbJob *models.CronJob
		var err error
		if dbJob, err = s.getJobRecord(ctx, jobID); err != nil {
			return fmt.Errorf("failed to get job record for retry: %w", err)
		}

		if len(dbJob.RetryPolicy) > 0 {
			var retryPolicy core.RetryPolicy
			if err := json.Unmarshal([]byte(dbJob.RetryPolicy), &retryPolicy); err != nil {
				return fmt.Errorf("failed to unmarshal retry policy: %w", err)
			}

			if retryPolicy.MaxRetries == 0 {
				return nil
			}

			delay = retryPolicy.InitialDelay * time.Duration(math.Pow(retryPolicy.BackoffFactor, float64(failures-1)))
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
		}
	}

	// Apply delay to job definition if needed
	if delay > 0 {
		jobDef = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(delay)))
	}

	// Define the before job runs event listener
	beforeJobRuns := func(jobID uuid.UUID, jobName string) {
		s.logger.Debug("Before job runs",
			zap.String("jobID", jobID.String()),
			zap.String("jobName", jobName))

		// Perform any pre-execution tasks here
		if err := s.SetupJob(jobCtx, jobID); err != nil {
			s.logger.Error("Failed to setup job",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
		}
	}

	// Define the after job runs event listener
	afterJobRuns := func(jobID uuid.UUID, jobName string) {
		s.logger.Debug("After job runs", zap.String("jobID", jobID.String()), zap.String("jobName", jobName))
		// Perform any post-execution tasks here
		if err = s.CleanupJob(jobCtx, jobID); err != nil {
			s.logger.Error("Failed to cleanup job", zap.String("jobID", jobID.String()), zap.Error(err))
		}
	}

	// Define the after job runs with error event listener
	afterJobRunsWithError := func(jobID uuid.UUID, jobName string, err error) {
		fields := []zap.Field{
			zap.String("jobID", jobID.String()),
			zap.String("jobName", jobName),
			zap.Error(err),
			zap.NamedError("ctxErr", jobCtx.Err()),
		}

		// Check if this error is from a recovered panic
		// gocron wraps panics as: "gocron: panic recovered from <panic value>"
		errStr := err.Error()
		panicPrefix := "gocron: panic recovered from "
		if strings.HasPrefix(errStr, panicPrefix) {
			// Extract the panic value
			panicValue := strings.TrimPrefix(errStr, panicPrefix)

			// Capture structured panic info using the panic value
			panicInfo := captureStructuredPanic(panicValue)

			// Add structured panic info to the error log
			fields = append(fields, buildPanicFields(panicInfo)...)
		}

		s.logger.Error("Job execution failed", fields...)

		s.failureMu.Lock()
		// Increment failure count
		s.failureCounts[jobID]++
		failures := s.failureCounts[jobID]
		s.failureMu.Unlock()

		// Handle all failure transitions through HandleFailedJob
		if err := s.HandleFailedJob(jobCtx, jobID, uint(failures)); err != nil {
			s.logger.Error("Failed to handle failed job",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
		}
	}

	// Define the after job runs with panic event listener
	afterJobRunsWithPanic := func(jobID uuid.UUID, jobName string, recoverData any) {
		// Capture structured panic info
		panicInfo := captureStructuredPanic(recoverData)

		// Build zap fields including job info and panic details
		fields := []zap.Field{
			zap.String("jobID", jobID.String()),
			zap.String("jobName", jobName),
		}
		fields = append(fields, buildPanicFields(panicInfo)...)

		s.logger.Error("Task panicked", fields...)

		s.failureMu.Lock()
		// Increment failure count
		s.failureCounts[jobID]++
		failures := s.failureCounts[jobID]
		s.failureMu.Unlock()

		// Handle all failure transitions through HandleFailedJob
		// For panic recovery, we'll treat it as a retryable failure (not permanent)
		if err := s.HandleFailedJob(jobCtx, jobID, uint(failures)); err != nil {
			s.logger.Error("Failed to handle failed job", zap.String("jobID", jobID.String()), zap.Error(err))
		}
	}

	// Create the job with the defined task and event listeners
	_, err = s.scheduler.Update(jobID,
		jobDef,
		gocron.NewTask(taskFunc, jobID),
		gocron.WithEventListeners(
			gocron.BeforeJobRuns(beforeJobRuns),
			gocron.AfterJobRuns(afterJobRuns),
			gocron.AfterJobRunsWithError(afterJobRunsWithError),
			gocron.AfterJobRunsWithPanic(afterJobRunsWithPanic),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}

	return nil
}

func (s *StandaloneCoordinator) Start() error {
	s.scheduler.Start()
	return nil
}

func (s *StandaloneCoordinator) HandleFailedJob(ctx context.Context, jobID uuid.UUID, failures uint) error {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.HandleFailedJob")
	defer span.End()

	// Cancel any existing context first
	s.cancelJobContext(jobID)

	// Stop heartbeat monitoring
	s.cronService.Monitor().StopHeartbeat(ctx, jobID)

	// Update job state to failed and increment failures
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	if err := s.cronService.StateMachine().Transition(
		ctx,
		jobID,
		models.CronJobStateFailed,
		core.WithCronFailures(int(failures)),
	); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// Determine if this is a permanent failure based on retry policy
	var retryPolicy *core.RetryPolicy
	if dbJob, err := s.getJobRecord(ctx, jobID); err == nil && len(dbJob.RetryPolicy) > 0 {
		if err := json.Unmarshal([]byte(dbJob.RetryPolicy), &retryPolicy); err != nil {
			s.logger.Error("Failed to parse retry policy",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
		}
	}

	maxFailures := s.maxFailures
	if retryPolicy != nil && retryPolicy.MaxRetries > 0 {
		maxFailures = retryPolicy.MaxRetries
	}

	permanent := int(failures) >= maxFailures

	// If this is a permanent failure, don't requeue
	if permanent {
		s.logger.Error("Job exceeded maximum failure threshold - marking as permanently failed",
			zap.String("jobID", jobID.String()),
			zap.Uint("failures", failures),
			zap.Int("maxFailures", maxFailures))
		return nil
	}

	// Reset the job to Queued before requeueing. SetupJob only cycles
	// Queued -> Running and CleanupJob only cycles Running -> Completed, so
	// if we left the job in Failed the retry would execute business logic but
	// never return to a completed state. Failed -> Queued is a valid FSM
	// transition that restores the normal lifecycle.
	if err := s.cronService.StateMachine().Transition(
		ctx,
		jobID,
		models.CronJobStateQueued,
		core.WithCronFailures(int(failures)),
	); err != nil {
		return fmt.Errorf("failed to reset job to queued state for retry: %w", err)
	}

	// Requeue the job for retry
	if err := s.EnqueueJob(ctx, jobID); err != nil {
		return fmt.Errorf("failed to requeue job: %w", err)
	}

	return nil
}

func (s *StandaloneCoordinator) getOrCreateJobContext(ctx context.Context, jobID uuid.UUID) (context.Context, error) {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.getOrCreateJobContext")
	defer span.End()

	s.jobCtxMu.Lock()
	defer s.jobCtxMu.Unlock()

	// Check if we have an existing valid context
	if jobCtx, exists := s.jobContexts[jobID]; exists {
		select {
		case <-jobCtx.Done():
			// Context is canceled, create new one
		default:
			// Existing context is still valid
			return jobCtx, nil
		}
	}

	// Create new context
	ctx, cancel := context.WithCancel(ctx)
	s.jobContexts[jobID] = ctx
	s.jobCancels[jobID] = cancel

	return ctx, nil
}

func (s *StandaloneCoordinator) SetupJob(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.SetupJob")
	defer span.End()

	// Get current job state
	dbJob, err := s.getJobRecord(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job record: %w", err)
	}

	// Already completed — nothing to set up. This can happen when the job's
	// Run() method transitions to Completed before the beforeJobRuns callback fires.
	if dbJob.State == models.CronJobStateCompleted {
		return nil
	}

	// Validate retry policy early to catch configuration errors before job execution
	if err := s.validateRetryPolicy(ctx, jobID); err != nil {
		return fmt.Errorf("retry policy validation failed: %w", err)
	}

	jobCtx, err := s.getOrCreateJobContext(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to create job context: %w", err)
	}

	// Only transition to running if currently queued
	if dbJob.State == models.CronJobStateQueued {
		if err := s.cronService.StateMachine().Transition(
			jobCtx,
			jobID,
			models.CronJobStateRunning,
			core.WithCronHeartbeat(),
			core.WithCronLastRun(),
		); err != nil {
			s.cancelJobContext(jobID)
			return fmt.Errorf("failed to transition job to running state: %w", err)
		}
	}

	// Start heartbeat monitoring
	s.cronService.Monitor().StartHeartbeat(ctx, jobID)

	s.logger.Info("Started job execution",
		zap.String("jobID", jobID.String()))
	return nil
}

func (s *StandaloneCoordinator) cancelJobContext(jobID uuid.UUID) {
	s.jobCtxMu.Lock()
	defer s.jobCtxMu.Unlock()

	if cancel, exists := s.jobCancels[jobID]; exists {
		cancel()
		delete(s.jobCancels, jobID)
		delete(s.jobContexts, jobID)
	}
}

func (s *StandaloneCoordinator) CleanupJob(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.CleanupJob")
	defer span.End()

	// Get current job state
	dbJob, err := s.getJobRecord(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job record: %w", err)
	}

	// Always stop the heartbeat first — it may have been started by SetupJob
	// before the job completed, and the goroutine will leak if not stopped.
	s.cronService.Monitor().StopHeartbeat(ctx, jobID)

	// Already completed — nothing else to clean up. This can happen when the job's
	// Run() method transitions to Completed before the afterJobRuns callback fires.
	if dbJob.State == models.CronJobStateCompleted {
		return nil
	}
	s.cancelJobContext(jobID)

	// Reset failure count on successful completion
	s.failureMu.Lock()
	delete(s.failureCounts, jobID)
	s.failureMu.Unlock()

	// Only transition to completed if currently running
	state := dbJob.State
	if state == models.CronJobStateRunning {
		if err = s.cronService.StateMachine().Transition(
			ctx,
			jobID,
			models.CronJobStateCompleted,
		); err != nil {
			return fmt.Errorf("failed to transition job to completed state: %w", err)
		}
		state = models.CronJobStateCompleted
	}

	// Remove completed job from scheduler only if it's a one-shot job
	if state == models.CronJobStateCompleted {
		// Check if this is a "once" job that should be removed from scheduler
		if dbJob.ScheduleType == string(core.CronScheduleTypeOnce) {
			if err := s.scheduler.RemoveJob(jobID); err != nil {
				s.logger.Warn("Failed to remove completed job from scheduler",
					zap.String("jobID", jobID.String()),
					zap.Error(err))
			}
		}
	}

	s.logger.Debug("Finished job execution",
		zap.String("jobID", jobID.String()))
	return nil
}

func (s *StandaloneCoordinator) ExecuteJob(ctx context.Context, jobID uuid.UUID) error {
	// Start a new root trace for each cron job execution. Cron jobs are
	// triggered by the scheduler (no HTTP parent), so they need their own
	// root span to act as the trace root for all downstream work.
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.ExecuteJob",
		core.WithNewRoot(), core.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	job, err := s.CreateJobFromDB(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to create job instance: %w", err)
	}

	if job == nil {
		return fmt.Errorf("failed to execute job: job instance is nil")
	}

	// Create a per-job-type OTel span so each execution is traceable by job type
	// (e.g. "cron.core.pin-checker") in addition to the generic coordinator span.
	// This is a child of the ExecuteJob root span, so all job work stays in
	// the same trace.
	jobCtx, jobSpan := core.TraceMethod(ctx, "cron."+job.Type())
	defer jobSpan.End()

	return job.Run(s.ctx, jobCtx)
}

func (s *StandaloneCoordinator) Jobs() []gocron.Job {
	return s.scheduler.Jobs()
}

func (s *StandaloneCoordinator) Close() error {
	return s.scheduler.Shutdown()
}

func (s *StandaloneCoordinator) RemoveJob(jobID uuid.UUID) error {
	return s.scheduler.RemoveJob(jobID)
}

func (s *StandaloneCoordinator) JobContext(ctx context.Context, jobID uuid.UUID) context.Context {
	ctx, span := core.TraceMethod(ctx, "StandaloneCoordinator.JobContext")
	defer span.End()

	s.jobCtxMu.RLock()
	defer s.jobCtxMu.RUnlock()
	return s.jobContexts[jobID]
}

// isInternalPath checks if a function path is internal framework code
func isInternalPath(function string) bool {
	internalPrefixes := []string{
		"github.com/go-co-op/gocron/",
		"go.lumeweb.com/portal/service/internal/cron.",
		"runtime.",
		"reflect.",
	}

	for _, prefix := range internalPrefixes {
		if strings.HasPrefix(function, prefix) {
			return true
		}
	}
	return false
}

// buildPanicFields builds structured zap fields from panic information.
// Returns fields for panic value, message, and filtered stack frames.
func buildPanicFields(panicInfo *PanicInfo) []zap.Field {
	fields := []zap.Field{
		zap.Any("panicValue", panicInfo.RecoverVal),
		zap.String("panicMessage", panicInfo.Message),
	}

	// Add stack frames if available
	if len(panicInfo.Stack) > 0 {
		for i, frame := range panicInfo.Stack {
			if i >= 5 {
				break
			}
			fields = append(fields,
				zap.String(fmt.Sprintf("frame%d", i), fmt.Sprintf("%s:%d (%s)", frame.File, frame.Line, frame.Function)))
		}

		if len(panicInfo.Stack) > 5 {
			fields = append(fields, zap.Int("additionalFrames", len(panicInfo.Stack)-5))
		}
	}

	return fields
}

// captureStructuredPanic captures structured panic information using Go's runtime API.
// Returns PanicInfo with the panic value and filtered stack frames.
func captureStructuredPanic(recoverData any) *PanicInfo {
	info := &PanicInfo{
		RecoverVal: recoverData,
	}

	// Format the panic message
	if err, ok := recoverData.(error); ok {
		info.Message = err.Error()
	} else {
		info.Message = fmt.Sprintf("%v", recoverData)
	}

	// Capture up to 64 stack frames
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs) // skip 2 frames: runtime.Callers and this function

	if n == 0 {
		return info
	}

	frames := runtime.CallersFrames(pcs[:n])

	// Collect non-internal frames
	for {
		frame, more := frames.Next()

		// Skip internal frames
		if !isInternalPath(frame.Function) {
			info.Stack = append(info.Stack, StackFrame{
				Function: frame.Function,
				File:     frame.File,
				Line:     frame.Line,
			})
		}

		if !more {
			break
		}
	}

	return info
}
