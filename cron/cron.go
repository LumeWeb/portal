package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"git.lumeweb.com/LumeWeb/portal/db/types"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/go-co-op/gocron/v2"
)

var (
	ErrRetryLimitReached = errors.New("Retry limit reached")
)

type TaskFunction func(any) error
type TaskArgsFactoryFunction func() any

type CronService interface {
	RegisterService(service CronableService)
	RegisterTask(name string, taskFunc TaskFunction, taskArgFunc TaskArgsFactoryFunction)
	CreateJob(function string, args any, tags []string) error
	JobExists(function string, args any, tags []string) (bool, *models.CronJob)
	CreateJobScheduled(function string, args any, tags []string, jobDef gocron.JobDefinition) error
	CreateExistingJobScheduled(uuid uuid.UUID, jobDef gocron.JobDefinition) error
	CreateJobIfNotExists(function string, args any, tags []string) error
}

type CronableService interface {
	RegisterTasks(cron CronService) error
	ScheduleJobs(cron CronService) error
}

type CronServiceParams struct {
	fx.In
	Logger    *zap.Logger
	Scheduler gocron.Scheduler
	Db        *gorm.DB
}

var Module = fx.Module("cron",
	fx.Options(
		fx.Provide(NewCronService),
		fx.Provide(gocron.NewScheduler),
	),
)

type CronServiceDefault struct {
	scheduler gocron.Scheduler
	services  []CronableService
	logger    *zap.Logger
	db        *gorm.DB
	tasks     sync.Map
	taskArgs  sync.Map
}

func NewCronService(lc fx.Lifecycle, params CronServiceParams) *CronServiceDefault {
	return &CronServiceDefault{
		logger:    params.Logger,
		scheduler: params.Scheduler,
		db:        params.Db,
	}
}

func Start(lc fx.Lifecycle, service *CronServiceDefault) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return service.start()
		},
	})

	return nil
}

func (c *CronServiceDefault) start() error {
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

func (c *CronServiceDefault) kickOffJob(job *models.CronJob, jobDef gocron.JobDefinition) error {
	argsFunc, ok := c.taskArgs.Load(job.Function)

	if !ok {
		return fmt.Errorf("function %s not found", job.Function)
	}

	args := argsFunc.(TaskArgsFactoryFunction)()

	if len(job.Args) > 0 {
		err := json.Unmarshal([]byte(job.Args), &args)
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
		jobDef = gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())
	}

	_, err := c.scheduler.NewJob(jobDef, task, options...)
	if err != nil {
		return err
	}

	return nil
}

func (c *CronServiceDefault) RegisterService(service CronableService) {
	c.services = append(c.services, service)
}

func (c *CronServiceDefault) RegisterTask(name string, taskFunc TaskFunction, taskArgFunc TaskArgsFactoryFunction) {
	c.tasks.Store(name, taskFunc)
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
