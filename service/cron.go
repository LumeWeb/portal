package service

import (
	"encoding/json"
	"fmt"
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
	"time"
)

var _ core.CronService = (*CronServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.CRON_SERVICE,
		Factory: NewCronService,
		Depends: []string{},
	})
}

type CronServiceDefault struct {
	ctx                core.Context
	db                 *gorm.DB
	coordinator        core.CronCoordinator
	jobFactory         core.CronJobFactory
	scheduleRegistry   core.CronScheduleRegistry
	logger             *core.Logger
	stateMachine       core.CronJobStateMachine
	stopHeartbeat      chan struct{}
	heartbeatTicker    *time.Ticker
	monitor            core.CronMonitor
	entities           []core.Cronable
	defaultRetryPolicy *core.RetryPolicy
}

func (c *CronServiceDefault) initializeComponents(ctx core.Context) error {
	c.ctx = ctx
	c.db = ctx.DB()
	c.logger = ctx.ServiceLogger(c)

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

	c.monitor = cron.NewDefaultCronMonitor(ctx, c)

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

func (c *CronServiceDefault) loadAndValidateJobs() error {
	if err := c.loadJobsFromDB(); err != nil {
		return fmt.Errorf("failed to load jobs from database: %w", err)
	}

	if err := c.registerMaintenanceJobs(); err != nil {
		return fmt.Errorf("failed to register maintenance jobs: %w", err)
	}

	if cleaned, err := c.monitor.CleanupOrphanedJobs(); err != nil {
		return fmt.Errorf("failed to validate existing jobs: %w", err)
	} else if cleaned > 0 {
		c.logger.Info("Cleaned up orphaned jobs", zap.Int("count", cleaned))
	}

	return nil
}

func NewCronService() (core.Service, []core.ContextBuilderOption, error) {
	cronService := &CronServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			cronService.ctx = ctx
			cronService.db = ctx.DB()
			cronService.logger = ctx.ServiceLogger(cronService)
			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return cronService.Stop()
		}),
	)

	return cronService, opts, nil
}

func (c *CronServiceDefault) ID() string {
	return core.CRON_SERVICE
}

func (c *CronServiceDefault) Start() error {
	err := c.initializeComponents(c.ctx)
	if err != nil {
		return err
	}

	for _, service := range c.entities {
		err := service.RegisterTasks(c)
		if err != nil {
			c.logger.Fatal("Failed to register tasks for service", zap.Error(err))
		}
	}

	if err = c.loadAndValidateJobs(); err != nil {
		return err
	}

	for _, service := range c.entities {
		err := service.ScheduleJobs(c)
		if err != nil {
			c.logger.Error("Failed to schedule jobs for service", zap.Error(err))
			return err
		}
	}

	if c.ctx.Config().Config().Core.Cron.Enabled {
		err := c.coordinator.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CronServiceDefault) registerMaintenanceJobs() error {
	// Register dead job check job using the pre-registered schedule
	err := c.RegisterJobType(core.GetCronJobIdentifier(core.JobOriginCore, cron.DeadJobCheckJobType), func() (core.CronJob, error) {
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
	err = c.RegisterJobType(core.GetCronJobIdentifier(core.JobOriginCore, cron.CleanupJobType), func() (core.CronJob, error) {
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

func (c *CronServiceDefault) Stop() error {
	return c.coordinator.Close()
}

func (c *CronServiceDefault) GetActiveJob(jobID uuid.UUID) (core.CronJob, bool, error) {
	// Check coordinator's active jobs first
	jobs := c.coordinator.Jobs()
	for _, job := range jobs {
		if job.ID() == jobID {
			// Found active job, create and return the instance
			cronJob, err := c.coordinator.CreateJobFromDB(jobID)
			if err != nil {
				return nil, false, fmt.Errorf("failed to create job instance: %w", err)
			}
			cronJob.SetJob(job)
			jobCtx := c.coordinator.JobContext(jobID)
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

func (c *CronServiceDefault) RegisterJob(job core.CronJob, retryPolicy *core.RetryPolicy) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if err := core.ValidateCronJob(job); err != nil {
		return err
	}

	// Check for existing job
	var existingJob models.CronJob
	if err := c.db.Where(&models.CronJob{UUID: types.FromUUID(job.ID())}).First(&existingJob).Error; err == nil {
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
		schedDef, _ = c.JobFactory().GetDefaultSchedule(job.Type())
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

	if err := c.db.Create(&cronJob).Error; err != nil {
		return fmt.Errorf("failed to create database record: %w", err)
	}

	// Delegate scheduling to coordinator if available
	if c.coordinator != nil {
		if err := c.coordinator.EnqueueJob(job.ID()); err != nil {
			return fmt.Errorf("failed to schedule job: %w", err)
		}
	} else {
		c.logger.Warn("Coordinator not initialized, job will be scheduled on next startup",
			zap.String("jobID", job.ID().String()))
	}

	return nil
}

func (c *CronServiceDefault) RunJob(id uuid.UUID) error {
	// Simply enqueue the job through the coordinator
	return c.coordinator.EnqueueJob(id)
}

func (s *CronServiceDefault) RegisterJobType(
	jobType string,
	factory core.CronJobFactoryFunc,
	defaultSchedule *core.CronScheduleDefinition,
) error {
	// Register the job factory
	if err := s.jobFactory.RegisterFactory(jobType, factory, defaultSchedule); err != nil {
		return fmt.Errorf("failed to register job factory: %w", err)
	}

	// If default schedule provided, create and register a default job instance
	if defaultSchedule != nil {
		job, err := factory()
		if err != nil {
			return fmt.Errorf("failed to create default job: %w", err)
		}
		return s.RegisterJob(job, defaultSchedule.RetryPolicy)
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

func (s *CronServiceDefault) RegisterPluginJobs(plugin core.PluginInfo) error {
	for _, jobReg := range plugin.CronJobs {
		// Use the existing GetCronJobIdentifier which handles proper formatting
		jobType := core.GetCronJobIdentifier(core.JobOriginPlugin, fmt.Sprintf("%s.%s", plugin.ID, jobReg.Name))
		err := s.RegisterJobType(
			jobType,
			jobReg.Factory,
			jobReg.Schedule,
		)
		if err != nil {
			return fmt.Errorf("failed to register job %s (type: %s): %w", jobReg.Name, jobType, err)
		}
	}
	return nil
}

func (c *CronServiceDefault) loadJobsFromDB() error {
	dbJobs := make([]models.CronJob, 0)
	if err := c.db.Where(models.CronJob{
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
		if err := c.coordinator.EnqueueJob(jobID); err != nil {
			c.logger.Error("Failed to schedule job from database",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
			continue
		}
	}

	return nil
}
