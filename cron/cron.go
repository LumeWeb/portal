package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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
}

type CronableService interface {
	RegisterTasks(cron CronService) error
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
	sc := &CronServiceDefault{
		logger:    params.Logger,
		scheduler: params.Scheduler,
		db:        params.Db,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return sc.start()
		},
	})

	return sc
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
		err := c.kickOffJob(cronJob)
		if err != nil {
			c.logger.Error("Failed to kick off job", zap.Error(err))
		}
	}

	go c.scheduler.Start()

	return nil
}

func (c *CronServiceDefault) kickOffJob(job models.CronJob) error {
	argsFunc, ok := c.taskArgs.Load(job.Function)

	if !ok {
		return fmt.Errorf("function %s not found", job.Function)
	}

	args := argsFunc.(TaskArgsFactoryFunction)()

	err := json.Unmarshal([]byte(job.Args), &args)
	if err != nil {
		return err
	}

	taskFunc, ok := c.tasks.Load(job.Function)

	if !ok {
		return fmt.Errorf("function %s not found", job.Function)
	}

	task := gocron.NewTask(taskFunc, args)

	options := []gocron.JobOption{}
	options = append(options, gocron.WithName(job.Name))
	options = append(options, gocron.WithTags(job.Tags...))
	options = append(options, gocron.WithIdentifier(job.UUID))

	listenerFunc := func(jobID uuid.UUID, jobName string, err error) {
		var job models.CronJob

		job.UUID = jobID
		if tx := c.db.Model(&models.CronJob{}).Delete(&job); tx.Error != nil {
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

	_, err = c.scheduler.NewJob(gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now())), task, options...)
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

func (c *CronServiceDefault) CreateJob(name string, tags []string, function string, args any) error {
	job := models.CronJob{
		UUID:     uuid.New(),
		Name:     name,
		Tags:     tags,
		Function: function,
	}

	bytes, err := json.Marshal(args)
	if err != nil {
		return err
	}

	job.Args = string(bytes)

	result := c.db.Create(&job)

	if result.Error != nil {
		return result.Error
	}

	return c.kickOffJob(job)
}
