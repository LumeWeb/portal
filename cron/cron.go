package cron

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/go-co-op/gocron/v2"
)

var (
	ErrRetryLimitReached = errors.New("Retry limit reached")
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

type CronServiceDefault struct {
	scheduler gocron.Scheduler
	services  []CronableService
	logger    *zap.Logger
}

type RetryableJobParams struct {
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

func (c *CronServiceDefault) Scheduler() gocron.Scheduler {
	return c.scheduler
}

func NewCronService(lc fx.Lifecycle, params CronServiceParams) *CronServiceDefault {
	sc := &CronServiceDefault{
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

func (c *CronServiceDefault) start() error {
	for _, service := range c.services {
		err := service.LoadInitialTasks(c)
		if err != nil {
			c.logger.Fatal("Failed to load initial tasks for service", zap.Error(err))
		}
	}

	go c.scheduler.Start()

	return nil
}

func (c *CronServiceDefault) RegisterService(service CronableService) {
	c.services = append(c.services, service)
}

func (c *CronServiceDefault) RetryableJob(params RetryableJobParams) CronJob {
	job := gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())

	if params.Attempt > 0 {
		job = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(time.Duration(params.Attempt) * time.Minute)))
	}

	task := gocron.NewTask(params.Function, params.Args...)

	if params.After == nil {
		params.After = func(jobID uuid.UUID, jobName string) {}
	}

	if params.Error == nil {
		params.Error = func(jobID uuid.UUID, jobName string, err error) {}
	}

	listeners := gocron.WithEventListeners(gocron.AfterJobRunsWithError(func(jobID uuid.UUID, jobName string, err error) {
		params.Error(jobID, jobName, err)

		if params.Attempt >= params.Limit && params.Limit > 0 {
			c.logger.Error("Retryable task limit reached", zap.String("jobName", jobName), zap.String("jobID", jobID.String()))
			params.Error(jobID, jobName, ErrRetryLimitReached)
			return
		}

		taskRetry := params
		taskRetry.Attempt++

		retryTask := c.RetryableJob(taskRetry)
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

func (c *CronServiceDefault) CreateJob(job CronJob) (gocron.Job, error) {
	ret, err := c.Scheduler().NewJob(job.Job, job.Task, job.Options...)
	if err != nil {
		return nil, err
	}

	return ret, nil
}
func (c *CronServiceDefault) RerunJob(job CronJob) (gocron.Job, error) {
	ret, err := c.Scheduler().Update(job.JobId, job.Job, job.Task, job.Options...)

	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (c *CronServiceDefault) GetJobsByPrefix(prefix string) []gocron.Job {
	jobs := c.Scheduler().Jobs()

	var ret []gocron.Job

	for _, job := range jobs {
		if strings.HasPrefix(job.Name(), prefix) {
			ret = append(ret, job)
		}
	}

	return ret
}

func (c *CronServiceDefault) GetJobByName(name string) gocron.Job {
	jobs := c.Scheduler().Jobs()

	for _, job := range jobs {
		if job.Name() == name {
			return job
		}
	}

	return nil
}

func (c *CronServiceDefault) GetJobByID(id uuid.UUID) gocron.Job {
	jobs := c.Scheduler().Jobs()

	for _, job := range jobs {
		if job.ID() == id {
			return job
		}
	}

	return nil
}
