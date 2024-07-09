package service

import (
	"encoding/json"
	"fmt"
	redislock "github.com/go-co-op/gocron-redis-lock/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"math"
	"math/rand"
	"sync"
	"time"
)

var _ core.CronService = (*CronServiceDefault)(nil)

const failureBackoffBaseDelay = 1 * time.Millisecond
const failureBackoffMaxDelay = 1 * time.Hour

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.CRON_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewCronService()
		},
	})
}

type CronServiceDefault struct {
	ctx           core.Context
	db            *gorm.DB
	logger        *core.Logger
	entities      []core.Cronable
	scheduler     gocron.Scheduler
	tasks         sync.Map
	taskArgs      sync.Map
	taskDefs      sync.Map
	taskRecurring sync.Map
}

func NewCronService() (*CronServiceDefault, []core.ContextBuilderOption, error) {
	cron := &CronServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			cron.ctx = ctx
			cron.db = ctx.DB()
			cron.logger = ctx.Logger()
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			scheduler, err := newScheduler(ctx.Config())
			if err != nil {
				return err
			}

			cron.scheduler = scheduler

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return cron.stop()
		}),
	)

	return cron, opts, nil
}

func newScheduler(cm config.Manager) (gocron.Scheduler, error) {
	cfg := cm.Config()
	if cfg.Core.ClusterEnabled() && cfg.Core.Clustered.Redis != nil {
		redisClient, err := cfg.Core.Clustered.Redis.Client()
		if err != nil {
			return nil, err
		}
		locker, err := redislock.NewRedisLocker(redisClient, redislock.WithTries(1), redislock.WithExpiry(time.Hour))
		if err != nil {
			return nil, err
		}

		return gocron.NewScheduler(gocron.WithDistributedLocker(locker))
	}

	return gocron.NewScheduler()
}
func (c *CronServiceDefault) Start() error {
	for _, service := range c.entities {
		err := service.RegisterTasks(c)
		if err != nil {
			c.logger.Fatal("Failed to register tasks for service", zap.Error(err))
		}
	}

	var cronJobs []models.CronJob
	result := c.db.Find(&cronJobs)
	if result.Error != nil {
		return result.Error
	}

	for _, cronJob := range cronJobs {
		err := c.kickOffJob(&cronJob, nil, cronJob.Failures)
		if err != nil {
			c.logger.Error("Failed to kick off job", zap.Error(err))
			return err
		}
	}

	for _, service := range c.entities {
		err := service.ScheduleJobs(c)
		if err != nil {
			c.logger.Error("Failed to schedule jobs for service", zap.Error(err))
			return err
		}
	}

	go c.scheduler.Start()

	return nil
}

func (c *CronServiceDefault) stop() error {
	err := c.scheduler.Shutdown()
	if err != nil {
		return err
	}
	return nil
}

func (c *CronServiceDefault) kickOffJob(job *models.CronJob, jobDef gocron.JobDefinition, errors uint) error {
	argsFunc, ok := c.taskArgs.Load(job.Function)

	if !ok {
		return fmt.Errorf("function %s not found", job.Function)
	}

	args := argsFunc.(core.CronTaskArgsFactoryFunction)()

	if len(job.Args) > 0 {
		err := json.Unmarshal([]byte(job.Args), args)
		if err != nil {
			return err
		}
	}

	taskFunc, ok := c.tasks.Load(job.Function)

	if !ok {
		return fmt.Errorf("function %s not found", job.Function)
	}

	varArgs := []interface{}{
		interface{}(struct{}{}),
		interface{}(c.ctx),
	}

	if args != nil {
		varArgs = []interface{}{
			args,
			interface{}(c.ctx),
		}
	}

	task := gocron.NewTask(taskFunc, varArgs...)

	options := []gocron.JobOption{}
	options = append(options, gocron.WithName(job.UUID.String()))
	options = append(options, gocron.WithIdentifier(uuid.UUID(job.UUID)))

	loadTaskDef := func(cronJob *models.CronJob) (def gocron.JobDefinition, err error) {
		if jobDef == nil {
			taskDefFunc, ok := c.taskDefs.Load(job.Function)

			if !ok {
				return nil, fmt.Errorf("function %s not found", job.Function)
			}

			jobDef = taskDefFunc.(core.CronTaskDefArgsFactoryFunction)()
		}

		return jobDef, nil
	}

	isRecurring := func(cronJob *models.CronJob) bool {
		_, ok := c.taskRecurring.Load(job.Function)
		return ok
	}

	updateLastRun := func(jobID uuid.UUID) error {
		var job models.CronJob

		job.UUID = types.BinaryUUID(jobID)
		lastRun := time.Now()
		if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Model(&models.CronJob{}).Where(&job).Updates(&models.CronJob{LastRun: &lastRun})
		}); err != nil {
			return err
		}
		return nil
	}

	listenerFuncErr := func(jobID uuid.UUID, jobName string, err error) {
		var job models.CronJob

		jobErr := err

		job.UUID = types.BinaryUUID(jobID)
		c.logger.Error("Job failed", zap.String("uuid", jobID.String()), zap.Error(err))

		if err = updateLastRun(jobID); err != nil {
			c.logger.Error("Failed to update last run", zap.Error(err))
		}

		if err = db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Model(&models.CronJob{}).Where(&job).Update("failures", gorm.Expr("failures + ?", 1))
		}); err != nil {
			c.logger.Error("Failed to update failures", zap.Error(err))
		}

		var updatedJob models.CronJob
		if err = db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Where(&job).First(&updatedJob)
		}); err != nil {
			c.logger.Error("Failed to fetch updated job", zap.Error(err))
			return
		}

		cronLog := &models.CronJobLog{
			CronJobID: job.ID,
			Type:      models.CronJobLogTypeFailure,
			Message:   jobErr.Error(),
		}

		if err = db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Create(cronLog)
		}); err != nil {
			c.logger.Error("Failed to create cron job log", zap.Error(err))
		}

		err = c.kickOffJob(&updatedJob, jobDef, updatedJob.Failures)
		if err != nil {
			c.logger.Error("Failed to kick off job", zap.Error(err))
		}
	}

	listenerFuncNoError := func(jobID uuid.UUID, jobName string) {
		var job models.CronJob

		job.UUID = types.BinaryUUID(jobID)
		if err := updateLastRun(jobID); err != nil {
			c.logger.Error("Failed to update last run", zap.Error(err))
		}

		if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Model(&models.CronJob{}).Where(&job).Update("failures", 0)
		}); err != nil {
			c.logger.Error("Failed to clear failures", zap.Error(err))
		}

		if isRecurring(&job) {
			originalJobDef, err := loadTaskDef(&job)
			if err != nil {
				c.logger.Error("Failed to load task definition", zap.Error(err))
				return
			}

			_, err = c.scheduler.Update(jobID, originalJobDef, task, options...)
			if err != nil {
				c.logger.Error("Failed to update job", zap.Error(err))
			}
		} else {
			if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.CronJob{}).Where(&job).Delete(&job)
			}); err != nil {
				c.logger.Error("Failed to delete job", zap.Error(err))
			}
		}
	}

	listeners := []gocron.EventListener{gocron.AfterJobRuns(listenerFuncNoError), gocron.AfterJobRunsWithError(listenerFuncErr)}

	options = append(options, gocron.WithEventListeners(listeners...))

	jobDefFinal, err := loadTaskDef(job)

	if errors > 0 {
		jobDefFinal = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(exponentialBackoff(errors, failureBackoffBaseDelay, failureBackoffMaxDelay))))
	}

	_, err = c.scheduler.NewJob(jobDefFinal, task, options...)
	if err != nil {
		return err
	}

	return nil
}

func (c *CronServiceDefault) RegisterEntity(service core.Cronable) {
	c.entities = append(c.entities, service)
}

func (c *CronServiceDefault) RegisterTask(name string, taskFunc core.CronTaskFunction, taskDefFunc core.CronTaskDefArgsFactoryFunction, taskArgFunc core.CronTaskArgsFactoryFunction, recurring bool) {
	c.tasks.Store(name, taskFunc)
	c.taskDefs.Store(name, taskDefFunc)
	c.taskArgs.Store(name, taskArgFunc)
	if recurring {
		c.taskRecurring.Store(name, recurring)
	}
}

func (c *CronServiceDefault) CreateJob(function string, args any) error {
	job, err := c.createJobRecord(function, args)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, nil, job.Failures)
}

func (c *CronServiceDefault) CreateJobScheduled(function string, args any, jobDef gocron.JobDefinition) error {
	job, err := c.createJobRecord(function, args)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, jobDef, job.Failures)
}

func (c *CronServiceDefault) CreateExistingJobScheduled(uuid uuid.UUID, jobDef gocron.JobDefinition) error {
	var job models.CronJob

	job.UUID = types.BinaryUUID(uuid)

	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.First(&job)
	}); err != nil {
		return err
	}

	return c.kickOffJob(&job, jobDef, job.Failures)
}

func (c *CronServiceDefault) CreateJobIfNotExists(function string, args any) error {
	exists, _ := c.JobExists(function, args)

	if !exists {
		return c.CreateJob(function, args)
	}

	return nil
}

func (c *CronServiceDefault) JobExists(function string, args any) (bool, *models.CronJob) {
	var job models.CronJob

	job.Function = function

	if args != nil {
		bytes, err := json.Marshal(args)
		if err != nil {
			return false, nil
		}

		job.Args = string(bytes)
	}

	result := c.db.Where(&job).First(&job)

	if result.Error != nil {
		return false, nil
	}

	return true, &job
}

func (c *CronServiceDefault) createJobRecord(function string, args any) (*models.CronJob, error) {
	job := models.CronJob{
		Function: function,
	}

	if args != nil {
		bytes, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}

		job.Args = string(bytes)
	}

	result := c.db.Create(&job)

	if result.Error != nil {
		return nil, result.Error
	}

	return &job, nil
}

func TaskDefinitionOneTimeJob() gocron.JobDefinition {
	return gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())
}
func exponentialBackoff(attempt uint, baseDelay, maxDelay time.Duration) time.Duration {
	// Calculate delay
	delay := float64(baseDelay) * math.Pow(2, float64(attempt))

	// Add jitter (randomness)
	jitter := rand.Float64() * 0.5 // 50% jitter
	delay = delay * (1 + jitter)

	// Cap the delay
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	return time.Duration(delay)
}
