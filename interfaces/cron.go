package interfaces

import "github.com/go-co-op/gocron/v2"

type CronService interface {
	Scheduler() gocron.Scheduler
	RegisterService(service CronableService)
	Service
}

type CronableService interface {
	LoadInitialTasks(cron CronService) error
	Service
}
