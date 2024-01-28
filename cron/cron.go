package cron

import (
	"context"
	"go.uber.org/fx"
	"go.uber.org/zap"

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
