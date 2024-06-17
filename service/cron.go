package service

import (
	"encoding/json"
	"fmt"
	redislock "github.com/go-co-op/gocron-redis-lock/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"sync"
	"time"
)

var _ core.CronService = (*CronServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.CRON_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewCronService()
		},
	})
}

type CronServiceDefault struct {
	ctx       core.Context
	db        *gorm.DB
	logger    *core.Logger
	services  []core.CronableService
	scheduler gocron.Scheduler
	tasks     sync.Map
	taskArgs  sync.Map
	taskDefs  sync.Map
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

			err = cron.Start(ctx)
			if err != nil {
				return err
			}

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return cron.Stop(ctx)
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
func (c *CronServiceDefault) Start(_ core.Context) error {
	for _, service := range c.services {
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
		err := c.kickOffJob(&cronJob, nil)
		if err != nil {
			c.logger.Error("Failed to kick off job", zap.Error(err))
			return err
		}
	}

	for _, service := range c.services {
		err := service.ScheduleJobs(c)
		if err != nil {
			c.logger.Error("Failed to schedule jobs for service", zap.Error(err))
			return err
		}
	}

	go c.scheduler.Start()

	return nil
}

func (c *CronServiceDefault) Init(_ core.Context) error {
	return nil
}

func (c *CronServiceDefault) Stop(_ core.Context) error {
	err := c.scheduler.Shutdown()
	if err != nil {
		return err
	}
	return nil
}

func (c *CronServiceDefault) kickOffJob(job *models.CronJob, jobDef gocron.JobDefinition) error {
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
	}

	if args != nil {
		varArgs = []interface{}{args}
	}

	task := gocron.NewTask(taskFunc, varArgs...)

	options := []gocron.JobOption{}
	options = append(options, gocron.WithName(job.UUID.String()))
	options = append(options, gocron.WithTags(job.Tags...))
	options = append(options, gocron.WithIdentifier(uuid.UUID(job.UUID)))

	listenerFunc := func(jobID uuid.UUID, jobName string, err error) {
		var job models.CronJob

		job.UUID = types.BinaryUUID(jobID)
		if tx := c.db.Model(&models.CronJob{}).Where(&job).Delete(&job); tx.Error != nil {
			c.logger.Error("Failed to delete job", zap.Error(tx.Error))
		}

		if err != nil {
			c.logger.Error("Job failed", zap.String("job", jobName), zap.String("id", jobID.String()), zap.Error(err))
		}
	}

	listenerFuncNoError := func(jobID uuid.UUID, jobName string) {
		listenerFunc(jobID, jobName, nil)
	}

	listeners := []gocron.EventListener{gocron.AfterJobRuns(listenerFuncNoError), gocron.AfterJobRunsWithError(listenerFunc)}

	options = append(options, gocron.WithEventListeners(listeners...))

	if jobDef == nil {
		taskDefFunc, ok := c.taskDefs.Load(job.Function)

		if !ok {
			return fmt.Errorf("function %s not found", job.Function)
		}

		jobDef = taskDefFunc.(core.CronTaskDefArgsFactoryFunction)()
	}

	_, err := c.scheduler.NewJob(jobDef, task, options...)
	if err != nil {
		return err
	}

	return nil
}

func (c *CronServiceDefault) RegisterService(service core.CronableService) {
	c.services = append(c.services, service)
}

func (c *CronServiceDefault) RegisterTask(name string, taskFunc core.CronTaskFunction, taskDefFunc core.CronTaskDefArgsFactoryFunction, taskArgFunc core.CronTaskArgsFactoryFunction) {
	c.tasks.Store(name, taskFunc)
	c.taskDefs.Store(name, taskDefFunc)
	c.taskArgs.Store(name, taskArgFunc)
}

func (c *CronServiceDefault) CreateJob(function string, args any, tags []string) error {
	job, err := c.createJobRecord(function, args, tags)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, nil)
}

func (c *CronServiceDefault) CreateJobScheduled(function string, args any, tags []string, jobDef gocron.JobDefinition) error {
	job, err := c.createJobRecord(function, args, tags)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, jobDef)
}

func (c *CronServiceDefault) CreateExistingJobScheduled(uuid uuid.UUID, jobDef gocron.JobDefinition) error {
	var job models.CronJob

	job.UUID = types.BinaryUUID(uuid)

	result := c.db.First(&job)

	if result.Error != nil {
		return result.Error
	}

	return c.kickOffJob(&job, jobDef)
}

func (c *CronServiceDefault) CreateJobIfNotExists(function string, args any, tags []string) error {
	exists, _ := c.JobExists(function, args, tags)

	if !exists {
		return c.CreateJob(function, args, tags)
	}

	return nil
}

func (c *CronServiceDefault) JobExists(function string, args any, tags []string) (bool, *models.CronJob) {
	var job models.CronJob

	if tags != nil {
		job.Tags = tags
	}

	job.Tags = tags
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

func (c *CronServiceDefault) createJobRecord(function string, args any, tags []string) (*models.CronJob, error) {
	job := models.CronJob{
		Tags:     tags,
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
