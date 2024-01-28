package cron

import (
	"context"
	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"time"

	"github.com/go-co-op/gocron/v2"
)

type CronService interface {
	Scheduler() gocron.Scheduler
	RegisterService(service CronableService)
}

type CronableService interface {
	LoadInitialTasks(cron CronService) error
}

type CronServiceParams struct {
	fx.In
	Logger    *zap.Logger
	Scheduler gocron.Scheduler
}

var Module = fx.Module("cron",
	fx.Options(
		fx.Provide(NewCronService),
		fx.Provide(gocron.NewScheduler),
	),
)

type CronServiceImpl struct {
	scheduler gocron.Scheduler
	services  []CronableService
	logger    *zap.Logger
}

type RetryableTaskParams struct {
	Name     string
	Tags     []string
	Function any
	Args     []any
	Attempt  uint
	Limit    uint
	After    func(jobID uuid.UUID, jobName string)
	Error    func(jobID uuid.UUID, jobName string, err error)
}

type CronJob struct {
	JobId   uuid.UUID
	Job     gocron.JobDefinition
	Task    gocron.Task
	Options []gocron.JobOption
}

func (c *CronServiceImpl) Scheduler() gocron.Scheduler {
	return c.scheduler
}

func NewCronService(lc fx.Lifecycle, params CronServiceParams) *CronServiceImpl {
	sc := &CronServiceImpl{
		logger:    params.Logger,
		scheduler: params.Scheduler,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return sc.start()
		},
	})

	return sc
}

func (c *CronServiceImpl) start() error {
	for _, service := range c.services {
		err := service.LoadInitialTasks(c)
		if err != nil {
			c.logger.Fatal("Failed to load initial tasks for service", zap.Error(err))
		}
	}

	go c.scheduler.Start()

	return nil
}

func (c *CronServiceImpl) RegisterService(service CronableService) {
	c.services = append(c.services, service)
}

func (c *CronServiceImpl) RetryableTask(params RetryableTaskParams) CronJob {
	job := gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())

	if params.Attempt > 0 {
		job = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(time.Duration(params.Attempt) * time.Minute)))
	}

	task := gocron.NewTask(params.Function, params.Args...)

	afterFunc := params.After
	if afterFunc == nil {
		afterFunc = func(jobID uuid.UUID, jobName string) {}
	}

	errorFunc := params.Error
	if errorFunc == nil {
		errorFunc = func(jobID uuid.UUID, jobName string, err error) {}
	}

	listeners := gocron.WithEventListeners(gocron.AfterJobRunsWithError(func(jobID uuid.UUID, jobName string, err error) {
		params.Error(jobID, jobName, err)

		if params.Attempt >= params.Limit {
			c.logger.Error("Retryable task limit reached", zap.String("jobName", jobName), zap.String("jobID", jobID.String()))
			return
		}

		taskRetry := params
		taskRetry.Attempt++

		retryTask := c.RetryableTask(taskRetry)
		retryTask.JobId = jobID

		_, err = c.RerunJob(retryTask)
		if err != nil {
			c.logger.Error("Failed to create retry job", zap.Error(err))
		}
	}), gocron.AfterJobRuns(params.After))

	name := gocron.WithName(params.Name)
	options := []gocron.JobOption{listeners, name}

	if len(params.Tags) > 0 {
		options = append(options, gocron.WithTags(params.Tags...))
	}

	return CronJob{
		Job:     job,
		Task:    task,
		Options: options,
	}
}

func (c *CronServiceImpl) CreateJob(job CronJob) (gocron.Job, error) {
	ret, err := c.Scheduler().NewJob(job.Job, job.Task, job.Options...)
	if err != nil {
		return nil, err
	}

	return ret, nil
}
func (c *CronServiceImpl) RerunJob(job CronJob) (gocron.Job, error) {
	ret, err := c.Scheduler().Update(job.JobId, job.Job, job.Task, job.Options...)

	if err != nil {
		return nil, err
	}

	return ret, nil
}
