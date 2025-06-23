package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"sync"
	"time"
)

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
		scheduler, err := gocron.NewScheduler()
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

func (s *StandaloneCoordinator) SetHeartbeat(jobID uuid.UUID) error {
	return s.stateMachine.Transition(s.ctx, jobID, models.CronJobStateRunning, core.WithCronHeartbeat())
}

func (s *StandaloneCoordinator) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	var job models.CronJob
	if err := s.db.Where("uuid = ?", types.FromUUID(jobID)).First(&job).Error; err != nil {
		return false, err
	}
	if job.LastHeartbeat == nil {
		return false, nil
	}
	return time.Since(*job.LastHeartbeat) < 2*time.Minute, nil
}

func (s *StandaloneCoordinator) CreateJobFromDB(jobID uuid.UUID) (core.CronJob, error) {
	return s.jobCreator.CreateFromDB(jobID)
}

func (s *StandaloneCoordinator) getJobRecord(jobID uuid.UUID) (*models.CronJob, error) {
	var dbJob models.CronJob
	err := s.db.Where(&models.CronJob{UUID: types.FromUUID(jobID)}).First(&dbJob).Error
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

func (s *StandaloneCoordinator) getScheduleDefinitionForJob(jobID uuid.UUID) (gocron.JobDefinition, error) {
	dbJob, err := s.getJobRecord(jobID)
	if err != nil {
		return nil, err
	}

	schedDef, err := s.getScheduleDefinition(dbJob)
	if err != nil {
		return nil, err
	}

	return s.cronService.ScheduleRegistry().Create(*schedDef)
}

func (s *StandaloneCoordinator) EnqueueJob(jobID uuid.UUID) error {
	// Validate jobID is not empty
	if jobID == uuid.Nil {
		return fmt.Errorf("invalid job ID: cannot be empty")
	}

	// Ensure we have a valid context before scheduling
	if _, err := s.getOrCreateJobContext(jobID); err != nil {
		return fmt.Errorf("failed to create job context: %w", err)
	}

	jobDef, err := s.getScheduleDefinitionForJob(jobID)
	if err != nil {
		return fmt.Errorf("failed to get schedule definition: %w", err)
	}

	// Define the task function
	taskFunc := func(jobID uuid.UUID) error {
		return s.ExecuteJob(jobID)
	}

	// Define the before job runs event listener
	beforeJobRuns := func(jobID uuid.UUID, jobName string) {
		s.logger.Debug("Before job runs", zap.String("jobID", jobID.String()), zap.String("jobName", jobName))
		// Perform any pre-execution tasks here
		if err := s.SetupJob(jobID); err != nil {
			s.logger.Error("Failed to setup job", zap.String("jobID", jobID.String()), zap.Error(err))
		}
	}

	// Define the after job runs event listener
	afterJobRuns := func(jobID uuid.UUID, jobName string) {
		s.logger.Debug("After job runs", zap.String("jobID", jobID.String()), zap.String("jobName", jobName))
		// Perform any post-execution tasks here
		if err = s.CleanupJob(jobID); err != nil {
			s.logger.Error("Failed to cleanup job", zap.String("jobID", jobID.String()), zap.Error(err))
		}
	}

	// Define the after job runs with error event listener
	afterJobRunsWithError := func(jobID uuid.UUID, jobName string, err error) {
		s.logger.Error("Job execution failed",
			zap.String("jobID", jobID.String()),
			zap.String("jobName", jobName),
			zap.Error(err))

		s.failureMu.Lock()
		defer s.failureMu.Unlock()

		// Increment failure count
		s.failureCounts[jobID]++

		if s.failureCounts[jobID] >= s.maxFailures {
			s.logger.Error("Job exceeded maximum failure threshold - marking as permanently failed",
				zap.String("jobID", jobID.String()),
				zap.Int("failures", s.failureCounts[jobID]))

			// Transition to permanent failure state
			if err := s.stateMachine.Transition(
				context.Background(),
				jobID,
				models.CronJobStateFailed,
				core.WithCronFailures(s.failureCounts[jobID]),
			); err != nil {
				s.logger.Error("Failed to transition job to failed state",
					zap.String("jobID", jobID.String()),
					zap.Error(err))
			}

			// Remove from scheduler
			if err := s.scheduler.RemoveJob(jobID); err != nil {
				s.logger.Error("Failed to remove failed job from scheduler",
					zap.String("jobID", jobID.String()),
					zap.Error(err))
			}

			// Clean up resources
			delete(s.failureCounts, jobID)
			s.cancelJobContext(jobID)
			return
		}

		// Standard failure handling
		if err := s.HandleFailedJob(jobID, uint(s.failureCounts[jobID])); err != nil {
			s.logger.Error("Failed to handle failed job",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
		}
	}

	// Define the after job runs with panic event listener
	afterJobRunsWithPanic := func(jobID uuid.UUID, jobName string, recoverData any) {
		s.logger.Error("Job runs with panic",
			zap.String("jobID", jobID.String()),
			zap.String("jobName", jobName),
			zap.Any("recoverData", recoverData))

		// Attempt to handle the failed job (e.g., requeue)
		var currentJob models.CronJob
		err = s.db.Where(&models.CronJob{UUID: types.FromUUID(jobID)}).First(&currentJob).Error
		if err != nil {
			s.logger.Error("Failed to get job failure count", zap.String("jobID", jobID.String()), zap.Error(err))
			return
		}
		if err = s.HandleFailedJob(jobID, currentJob.Failures+1); err != nil {
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

func (s *StandaloneCoordinator) HandleFailedJob(jobID uuid.UUID, failures uint) error {
	// Cancel any existing context first
	s.cancelJobContext(jobID)

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

	// Requeue the job
	if err := s.EnqueueJob(jobID); err != nil {
		return fmt.Errorf("failed to requeue job: %w", err)
	}

	return nil
}

func (s *StandaloneCoordinator) getOrCreateJobContext(jobID uuid.UUID) (context.Context, error) {
	s.jobCtxMu.Lock()
	defer s.jobCtxMu.Unlock()

	// Check if we have an existing valid context
	if ctx, exists := s.jobContexts[jobID]; exists {
		select {
		case <-ctx.Done():
			// Context is canceled, create new one
		default:
			// Existing context is still valid
			return ctx, nil
		}
	}

	// Create new context
	ctx, cancel := context.WithCancel(s.ctx)
	s.jobContexts[jobID] = ctx
	s.jobCancels[jobID] = cancel

	return ctx, nil
}

func (s *StandaloneCoordinator) SetupJob(jobID uuid.UUID) error {
	ctx, err := s.getOrCreateJobContext(jobID)
	if err != nil {
		return fmt.Errorf("failed to create job context: %w", err)
	}

	// Setup job state to Running
	if err := s.cronService.StateMachine().Transition(
		ctx,
		jobID,
		models.CronJobStateRunning,
		core.WithCronHeartbeat(),
		core.WithCronLastRun(),
	); err != nil {
		s.cancelJobContext(jobID)
		return fmt.Errorf("failed to transition job to running state: %w", err)
	}

	// Start heartbeat monitoring
	s.cronService.Monitor().StartHeartbeat(jobID)

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

func (s *StandaloneCoordinator) CleanupJob(jobID uuid.UUID) error {
	s.cronService.Monitor().StopHeartbeat(jobID)
	s.cancelJobContext(jobID)

	// Reset failure count on successful completion
	s.failureMu.Lock()
	delete(s.failureCounts, jobID)
	s.failureMu.Unlock()

	// Success case
	if err := s.cronService.StateMachine().Transition(
		context.Background(),
		jobID,
		models.CronJobStateCompleted,
	); err != nil {
		return fmt.Errorf("failed to transition job to completed state: %w", err)
	}

	s.logger.Debug("Finished job execution",
		zap.String("jobID", jobID.String()))
	return nil
}

func (s *StandaloneCoordinator) ExecuteJob(jobID uuid.UUID) error {
	// Get job instance
	job, err := s.CreateJobFromDB(jobID)
	if err != nil {
		return fmt.Errorf("failed to create job instance: %w", err)
	}

	// Execute the job logic
	if job == nil {
		return fmt.Errorf("failed to execute job: job instance is nil")
	}
	return job.Run(s.ctx)
}

func (s *StandaloneCoordinator) Jobs() []gocron.Job {
	return s.scheduler.Jobs()
}

func (s *StandaloneCoordinator) Close() error {
	return s.scheduler.Shutdown()
}

func (s *StandaloneCoordinator) JobContext(jobID uuid.UUID) context.Context {
	s.jobCtxMu.RLock()
	defer s.jobCtxMu.RUnlock()
	return s.jobContexts[jobID]
}
