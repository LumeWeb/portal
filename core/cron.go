package core

import (
	"github.com/LumeWeb/portal/db/models"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

type CronTaskFunction func(any, Context) error
type CronTaskArgsFactoryFunction func() any
type CronTaskDefArgsFactoryFunction func() gocron.JobDefinition

type CronService interface {
	RegisterService(service CronableService)
	RegisterTask(name string, taskFunc CronTaskFunction, taskDefFunc CronTaskDefArgsFactoryFunction, taskArgFunc CronTaskArgsFactoryFunction)
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

type CronTaskNoArgs struct{}

func CronTaskDefinitionOneTimeJob() gocron.JobDefinition {
	return gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())
}
func CronTaskNoArgsFactory() any {
	return &CronTaskNoArgs{}
}