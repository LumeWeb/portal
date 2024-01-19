package cron

import (
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"go.uber.org/zap"

	"github.com/go-co-op/gocron/v2"
)

var (
	_ interfaces.CronService = (*CronServiceImpl)(nil)
)

type CronServiceImpl struct {
	scheduler gocron.Scheduler
	services  []interfaces.CronableService
	portal    interfaces.Portal
}

func (c *CronServiceImpl) Scheduler() gocron.Scheduler {
	return c.scheduler
}

func NewCronServiceImpl(portal interfaces.Portal) interfaces.CronService {
	return &CronServiceImpl{}
}

func (c *CronServiceImpl) Init() error {
	s, err := gocron.NewScheduler()
	if err != nil {
		return err
	}

	c.scheduler = s

	return nil
}

func (c *CronServiceImpl) Start() error {
	for _, service := range c.services {
		err := service.LoadInitialTasks(c)
		if err != nil {
			c.portal.Logger().Fatal("Failed to load initial tasks for service", zap.Error(err))
		}
	}

	return nil
}

func (c *CronServiceImpl) RegisterService(service interfaces.CronableService) {
	c.services = append(c.services, service)
}
