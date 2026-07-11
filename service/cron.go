package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service/internal/cron"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var _ core.CronService = (*CronServiceDefault)(nil)

var (
	NewJobFactory              = cron.NewJobFactory
	NewScheduleRegistry        = cron.NewScheduleRegistry
	NewCronJobStateMachine     = cron.NewCronJobStateMachine
	NewStateMachineRegistry    = cron.NewStateMachineRegistry
	NewDefaultCronMonitor      = cron.NewDefaultCronMonitor
	NewStandaloneCoordinator   = cron.NewStandaloneCoordinator
	NewCoordinatorOptions      = cron.NewCoordinatorOptions
	NewCoordinatorFromContext  = cron.NewCoordinatorFromContext
	NewJobCreator              = cron.NewJobCreator

	// ErrCronJobNotFound is returned when a cron job cannot be found.
	ErrCronJobNotFound = cron.ErrCronJobNotFound
	// ErrCronJobVersionConflict is returned when a state transition fails due to optimistic locking.
	ErrCronJobVersionConflict = cron.ErrCronJobVersionConflict
)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.CRON_SERVICE,
		Factory: NewCronService,
		Depends: []string{},
		Metrics: cron.GetCollectors(),
	})
}

type CronServiceDefault struct {
	*core.BaseComponent
	coordinator        core.CronCoordinator
	jobFactory         core.CronJobFactory
	scheduleRegistry   core.CronScheduleRegistry
	stateMachine       core.CronJobStateMachine
	stopHeartbeat      chan struct{}
	heartbeatTicker    *time.Ticker
	monitor            core.CronMonitor
	entities           []core.Cronable
	defaultRetryPolicy *core.RetryPolicy
}

func (c *CronServiceDefault) initializeComponents(ctx core.Context) error {
	var stateMachineRegistry core.CronJobStateMachineRegistry
	var err error

	if stateMachineRegistry, err = c.initializeStateMachine(ctx); err != nil {
		return err
	}

	if err = c.initializeFactories(); err != nil {
		return err
	}

	if err = c.initializeCoordinator(ctx, stateMachineRegistry); err != nil {
		return err
	}

	if c.monitor == nil {
		c.monitor = cron.NewDefaultCronMonitor(ctx, c)
	}

	return nil
}

func (c *CronServiceDefault) initializeStateMachine(ctx core.Context) (core.CronJobStateMachineRegistry, error) {
	stateMachineRegistry := cron.NewStateMachineRegistry(ctx)
	c.stateMachine = cron.NewCronJobStateMachine(ctx, stateMachineRegistry)
	return stateMachineRegistry, nil
}

func (c *CronServiceDefault) initializeFactories() error {
	c.scheduleRegistry = cron.NewScheduleRegistry()
	c.jobFactory = cron.NewJobFactory(c.scheduleRegistry)
	return nil
}

func (c *CronServiceDefault) initializeCoordinator(ctx core.Context, registry core.CronJobStateMachineRegistry) error {
	coordinator, err := cron.NewCoordinatorFromContext(ctx, c, registry)
	if err != nil {
		return fmt.Errorf("failed to create coordinator: %w", err)
	}
	c.coordinator = coordinator
	return nil
}

func (c *CronServiceDefault) loadAndValidateJobs(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.loadAndValidateJobs")
	defer span.End()

	if err := c.loadJobsFromDB(ctx); err != nil {
		return fmt.Errorf("failed to load jobs from database: %w", err)
	}

	if err := c.registerMaintenanceJobs(ctx); err != nil {
		return fmt.Errorf("failed to register maintenance jobs: %w", err)
	}

	if cleaned, err := c.monitor.CleanupOrphanedJobs(ctx); err != nil {
		return fmt.Errorf("failed to validate existing jobs: %w", err)
	} else if cleaned > 0 {
		c.Logger().Info("Cleaned up orphaned jobs", zap.Int("count", cleaned))
	}

	return nil
}

func NewCronService() (core.Service, []core.ContextBuilderOption, error) {
	cronService := &CronServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return cronService.Stop(ctx)
		}),
	)

	return cronService, opts, nil
}

// NewTestingCronService creates a bare-bones CronService for testing purposes
func NewTestingCronService(
	ctx core.Context,
	db *gorm.DB,
	coordinator core.CronCoordinator,
	jobFactory core.CronJobFactory,
	scheduleRegistry core.CronScheduleRegistry,
	stateMachine core.CronJobStateMachine,
	monitor core.CronMonitor,
) core.CronService {
	cs := &CronServiceDefault{
		BaseComponent:    core.NewBaseComponent(ctx),
		coordinator:      coordinator,
		jobFactory:       jobFactory,
		scheduleRegistry: scheduleRegistry,
		stateMachine:     stateMachine,
	}
	if db != nil {
		cs.SetDB(db)
	}
	if cs.scheduleRegistry == nil {
		cs.scheduleRegistry = cron.NewScheduleRegistry()
	}
	if cs.jobFactory == nil {
		cs.jobFactory = cron.NewJobFactory(cs.scheduleRegistry)
	}
	if cs.stateMachine == nil {
		reg := cron.NewStateMachineRegistry(ctx)
		cs.stateMachine = cron.NewCronJobStateMachine(ctx, reg)
	}
	if monitor == nil {
		cs.monitor = cron.NewDefaultCronMonitor(ctx, cs)
	} else {
		cs.monitor = monitor
	}
	return cs
}

func (c *CronServiceDefault) ID() string {
	return core.CRON_SERVICE
}

func (c *CronServiceDefault) Start(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.Start")
	defer span.End()

	// If testing injected a full dependency set, don't overwrite it.
	if c.coordinator == nil || c.jobFactory == nil || c.scheduleRegistry == nil || c.stateMachine == nil || c.monitor == nil {
		if err := c.initializeComponents(c.Context()); err != nil {
			return err
		}
	} else {
		// Ensure core fields are wired from context.
		if c.DB() == nil {
			c.SetDB(c.Context().DB())
		}
		if c.Logger() == nil {
			c.SetLogger(c.Context().ServiceLogger(c))
		}
	}

	for _, service := range c.entities {
		err := service.RegisterTasks(ctx, c)
		if err != nil {
			c.Logger().Fatal("Failed to register tasks for service", zap.Error(err))
		}
	}

	// Register plugin cron jobs
	plugins := core.GetPlugins()
	for _, plugin := range plugins {
		if core.PluginHasCron(plugin) {
			err := c.RegisterPluginJobs(ctx, plugin)
			if err != nil {
				c.Logger().Fatal("Failed to register plugin cron jobs",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
			}
		}
	}

	if err := c.loadAndValidateJobs(ctx); err != nil {
		return err
	}

	for _, service := range c.entities {
		err := service.ScheduleJobs(ctx, c)
		if err != nil {
			c.Logger().Error("Failed to schedule jobs for service", zap.Error(err))
			return err
		}
	}

	if c.Config().Config().Core.Cron.Enabled {
		err := c.coordinator.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CronServiceDefault) registerMaintenanceJobs(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.registerMaintenanceJobs")
	defer span.End()

	// Register dead job check job using the pre-registered schedule
	err := c.RegisterJobType(ctx, core.GetCronJobIdentifier(core.JobOriginCore, cron.DeadJobCheckJobType), func() (core.CronJob, error) {
		return &cron.DeadJobCheckJob{
			BaseCronJob: core.NewBaseCronJob(
				uuid.New(),
				core.JobOriginCore,
				cron.DeadJobCheckJobType,
				"Dead Job Check",
				nil,
				nil,
			),
		}, nil
	}, &core.CronScheduleDefinition{
		Type: core.CronScheduleTypeDaily,
	})
	if err != nil {
		return fmt.Errorf("failed to register dead job check job: %w", err)
	}

	// Register cleanup job using the pre-registered schedule
	err = c.RegisterJobType(ctx, core.GetCronJobIdentifier(core.JobOriginCore, cron.CleanupJobType), func() (core.CronJob, error) {
		return &cron.CleanupJob{
			BaseCronJob: core.NewBaseCronJob(
				uuid.New(),
				core.JobOriginCore,
				cron.CleanupJobType,
				"Cleanup Job",
				nil,
				nil,
			),
		}, nil
	}, &core.CronScheduleDefinition{
		Type: core.CronScheduleTypeDaily,
	})
	if err != nil {
		return fmt.Errorf("failed to register cleanup job: %w", err)
	}

	return nil
}

func (c *CronServiceDefault) Monitor() core.CronMonitor {
	return c.monitor
}

func (c *CronServiceDefault) Stop(context.Context) error {
	if c.BaseComponent == nil {
		return nil
	}
	if c.Config().Config().Core.Cron.Enabled && c.coordinator != nil {
		return c.coordinator.Close()
	}

	return nil
}

func (c *CronServiceDefault) GetActiveJob(ctx context.Context, jobID uuid.UUID) (core.CronJob, bool, error) {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.GetActiveJob")
	defer span.End()

	// Check coordinator's active jobs first
	jobs := c.coordinator.Jobs()
	for _, job := range jobs {
		if job.ID() == jobID {
			// Found active job, create and return the instance
			cronJob, err := c.coordinator.CreateJobFromDB(ctx, jobID)
			if err != nil {
				return nil, false, fmt.Errorf("failed to create job instance: %w", err)
			}
			cronJob.SetJob(job)
			jobCtx := c.coordinator.JobContext(ctx, jobID)
			if jobCtx != nil {
				cronJob.SetDone(jobCtx.Done())

			}
			return cronJob, true, nil
		}
	}

	// Not found in active jobs
	return nil, false, nil
}

func (c *CronServiceDefault) RegisterEntity(service core.Cronable) {
	c.entities = append(c.entities, service)
}

func (c *CronServiceDefault) Coordinator() core.CronCoordinator {
	return c.coordinator
}

func (c *CronServiceDefault) SetCoordinator(coordinator core.CronCoordinator) {
	c.coordinator = coordinator
}

func (c *CronServiceDefault) RegisterJob(ctx context.Context, job core.CronJob, retryPolicy *core.RetryPolicy) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.RegisterJob")
	defer span.End()

	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if err := core.ValidateCronJob(job); err != nil {
		return err
	}

	// Check for existing job
	var existingJob models.CronJob
	if err := c.DB().Where(&models.CronJob{UUID: types.FromUUID(job.ID())}).First(&existingJob).Error; err == nil {
		return fmt.Errorf("job with ID %s already exists", job.ID())
	}

	// Serialize the arguments and retry policy
	var argsBytes []byte
	if job.Args() != nil {
		var err error
		argsBytes, err = json.Marshal(job.Args())
		if err != nil {
			return fmt.Errorf("failed to marshal arguments: %w", err)
		}
		// If args marshals to "null", treat as empty
		if string(argsBytes) == "null" {
			argsBytes = nil
		}
	}

	// Use provided retry policy or default if none specified
	if retryPolicy == nil {
		retryPolicy = core.DefaultRetryPolicy
	}

	var retryPolicyBytes []byte
	var err error
	retryPolicyBytes, err = json.Marshal(retryPolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal retry policy: %w", err)
	}

	// Get schedule definition from job
	var schedDefBytes []byte
	var schedDef *core.CronScheduleDefinition
	if schedDef = job.Schedule(); schedDef == nil {
		schedDef, _ = c.JobFactory().GetDefaultSchedule(ctx, job.Type())
	}

	if schedDef != nil {
		var err error
		schedDefBytes, err = json.Marshal(schedDef)
		if err != nil {
			return fmt.Errorf("failed to marshal schedule definition: %w", err)
		}
	}

	// Create the database record
	cronJob := models.CronJob{
		UUID:        types.FromUUID(job.ID()),
		Origin:      job.Origin(),
		SourceID:    job.SourceID(),
		JobType:     job.Type(),
		Args:        datatypes.JSON(argsBytes),
		RetryPolicy: datatypes.JSON(retryPolicyBytes),
		SchedDef:    datatypes.JSON(schedDefBytes),
		State:       models.CronJobStateQueued,
		Version:     1,
	}

	if err := c.DB().Create(&cronJob).Error; err != nil {
		return fmt.Errorf("failed to create database record: %w", err)
	}

	// Delegate scheduling to coordinator if available
	if c.coordinator != nil {
		if err := c.coordinator.EnqueueJob(ctx, job.ID()); err != nil {
			return fmt.Errorf("failed to schedule job: %w", err)
		}
	} else {
		c.Logger().Warn("Coordinator not initialized, job will be scheduled on next startup",
			zap.String("jobID", job.ID().String()))
	}

	return nil
}

func (c *CronServiceDefault) RunJob(ctx context.Context, id uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.RunJob")
	defer span.End()

	// Simply enqueue the job through the coordinator
	return c.coordinator.EnqueueJob(ctx, id)
}

func (s *CronServiceDefault) RegisterJobType(ctx context.Context, jobType string, factory core.CronJobFactoryFunc, defaultSchedule *core.CronScheduleDefinition) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.RegisterJobType")
	defer span.End()

	// Register the job factory
	if err := s.jobFactory.RegisterFactory(ctx, jobType, factory, defaultSchedule); err != nil {
		return fmt.Errorf("failed to register job factory: %w", err)
	}

	// If default schedule provided, create and register a default job instance
	if defaultSchedule != nil {
		job, err := factory()
		if err != nil {
			return fmt.Errorf("failed to create default job: %w", err)
		}
		return s.RegisterJob(ctx, job, defaultSchedule.RetryPolicy)
	}
	return nil
}

func (s *CronServiceDefault) ScheduleRegistry() core.CronScheduleRegistry {
	return s.scheduleRegistry
}

func (s *CronServiceDefault) JobFactory() core.CronJobFactory {
	return s.jobFactory
}

func (s *CronServiceDefault) StateMachine() core.CronJobStateMachine {
	return s.stateMachine
}

func (s *CronServiceDefault) RegisterPluginJobs(ctx context.Context, plugin core.PluginInfo) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.RegisterPluginJobs")
	defer span.End()

	for _, jobReg := range plugin.CronJobs {
		// Use the existing GetCronJobIdentifier which handles proper formatting
		jobType := core.GetCronJobIdentifier(core.JobOriginPlugin, fmt.Sprintf("%s.%s", plugin.ID, jobReg.Name))
		err := s.RegisterJobType(ctx, jobType, jobReg.Factory, jobReg.Schedule)
		if err != nil {
			return fmt.Errorf("failed to register job %s (type: %s): %w", jobReg.Name, jobType, err)
		}
	}
	return nil
}

func (c *CronServiceDefault) loadJobsFromDB(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "CronServiceDefault.loadJobsFromDB")
	defer span.End()

	dbJobs := make([]models.CronJob, 0)
	if err := c.DB().Where(models.CronJob{
		State: models.CronJobStateQueued,
	}).Find(&dbJobs).Error; err != nil {
		return fmt.Errorf("failed to load jobs from database: %w", err)
	}

	for _, dbJob := range dbJobs {
		jobID := dbJob.UUID.ToUUID()

		// Skip if already scheduled by checking all jobs' UUIDs
		jobs := c.coordinator.Jobs()

		if lo.ContainsBy(jobs, func(job gocron.Job) bool {
			return job.ID() == jobID
		}) {
			continue
		}

		// Delegate all job scheduling to coordinator
		if err := c.coordinator.EnqueueJob(ctx, jobID); err != nil {
			c.Logger().Error("Failed to schedule job from database",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
			continue
		}
	}

	return nil
}
