package cron

import (
	"context"
	"fmt"
	"go.lumeweb.com/portal/core"
	"sync"
)

const (
	DeadJobCheckJobType    = "cron.dead_job_check"
	CleanupJobType         = "cron.cleanup"
	IntegrationTestJobType = "core.cron.integration-test-job"
)

var _ core.CronJobFactory = (*DefaultJobFactory)(nil)

// DefaultJobFactory creates CronJob instances and manages job type registration
type DefaultJobFactory struct {
	factories      map[string]core.CronJobFactoryFunc
	defaultConfigs map[string]*core.CronScheduleDefinition
	schedRegistry  core.CronScheduleRegistry
	mu             *sync.RWMutex
}

func NewJobFactory(schedRegistry core.CronScheduleRegistry) *DefaultJobFactory {
	f := &DefaultJobFactory{
		factories:      make(map[string]core.CronJobFactoryFunc),
		defaultConfigs: make(map[string]*core.CronScheduleDefinition),
		schedRegistry:  schedRegistry,
		mu:             &sync.RWMutex{},
	}
	return f
}

var _ core.CronJob = (*DeadJobCheckJob)(nil)

type DeadJobCheckJob struct {
	*core.BaseCronJob
}

func (j *DeadJobCheckJob) Run(ctx core.Context, eventCtx context.Context) error {
	// Signal the monitor to perform maintenance
	cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
	cronService.Monitor().SignalMaintenance(nil)
	return nil
}

var _ core.CronJob = (*CleanupJob)(nil)

type CleanupJob struct {
	*core.BaseCronJob
}

func (j *CleanupJob) Run(ctx core.Context, eventCtx context.Context) error {
	cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
	return cronService.Monitor().CleanupCompletedJobs(nil)
}

// RegisterFactory registers a job type with optional default schedule
func (f *DefaultJobFactory) GetDefaultSchedule(ctx context.Context, jobType string) (*core.CronScheduleDefinition, bool) {
	ctx, span := core.TraceMethod(ctx, "DefaultJobFactory.GetDefaultSchedule")
	defer span.End()

	f.mu.RLock()
	defer f.mu.RUnlock()

	sched, exists := f.defaultConfigs[jobType]
	return sched, exists
}

func (f *DefaultJobFactory) RegisterFactory(ctx context.Context, jobType string, factory core.CronJobFactoryFunc, defaultSchedule *core.CronScheduleDefinition) error {
	ctx, span := core.TraceMethod(ctx, "DefaultJobFactory.RegisterFactory")
	defer span.End()

	if err := core.ValidateCronJobType(jobType); err != nil {
		return fmt.Errorf("invalid job type: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.factories[jobType]; exists {
		return fmt.Errorf("job type '%s' already registered", jobType)
	}

	f.factories[jobType] = factory
	if defaultSchedule != nil {
		f.defaultConfigs[jobType] = defaultSchedule
	}
	return nil
}

// CreateJob instantiates a job of the given type
func (f *DefaultJobFactory) CreateJob(ctx context.Context, jobType string) (core.CronJob, error) {
	ctx, span := core.TraceMethod(ctx, "DefaultJobFactory.CreateJob")
	defer span.End()

	f.mu.RLock()
	defer f.mu.RUnlock()

	factory, ok := f.factories[jobType]
	if !ok {
		return nil, fmt.Errorf("unknown job type: %s", jobType)
	}
	return factory()
}
