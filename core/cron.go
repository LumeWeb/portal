package core

import (
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/db/models"
)

type CronTaskFunction[T CronTaskArgs] func(T, Context) error
type CronTaskArgsFactoryFunction func() any
type CronTaskDefArgsFactoryFunction func() gocron.JobDefinition

const CRON_SERVICE = "cron"

type CronTaskArgs interface{}

type CronService interface {
	RegisterEntity(entity Cronable)
	RegisterTask(name string, taskFunc CronTaskFunction[CronTaskArgs], taskDefFunc CronTaskDefArgsFactoryFunction, taskArgFunc CronTaskArgsFactoryFunction, recurring bool)
	CreateJob(function string, args any) error
	JobExists(function string, args any) (bool, *models.CronJob)
	CreateJobScheduled(function string, args any) error
	CreateExistingJobScheduled(uuid uuid.UUID) error
	CreateJobIfNotExists(function string, args any) error

	Start() error
	Service
}
type Cronable interface {
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

func PluginHasCron(pi PluginInfo) bool {
	return pi.Cron != nil
}

func CronTaskFuncHandler[T CronTaskArgs](f func(args T, ctx Context) error) CronTaskFunction[CronTaskArgs] {
	return func(args CronTaskArgs, ctx Context) error {
		typedArgs, ok := args.(T)
		if !ok {
			return fmt.Errorf("invalid argument type: expected %T, got %T", *new(T), args)
		}
		return f(typedArgs, ctx)
	}
}
