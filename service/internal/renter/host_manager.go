package renter

import (
	"github.com/google/uuid"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
)

const (
	cronTaskScanHostsName = "ScanSiaHosts"
)

type HostManager struct {
	ctx     core.Context
	config  config.Manager
	renter  core.RenterService
	scanner *HostScanner
	cron    core.CronService
}

var _ core.Cronable = (*HostManager)(nil)

func NewHostManager(ctx core.Context) *HostManager {
	return &HostManager{
		ctx:     ctx,
		config:  ctx.Config(),
		renter:  core.GetService[core.RenterService](ctx, core.RENTER_SERVICE),
		scanner: NewHostScanner(ctx),
		cron:    core.GetService[core.CronService](ctx, core.CRON_SERVICE),
	}
}

func (t *HostManager) Init() error {
	t.cron.RegisterEntity(t)
	return nil
}

func (t *HostManager) RegisterTasks(crn core.CronService) error {
	crn.RegisterTask(cronTaskScanHostsName, core.CronTaskFuncHandler(t.updateHosts), core.CronTaskDefinitionDaily, core.CronTaskNoArgsFactory, true)
	return nil
}

func (t *HostManager) ScheduleJobs(crn core.CronService) error {
	exists, scanJobItem := crn.JobExists(cronTaskScanHostsName, nil)

	if !exists {
		err := crn.CreateJobScheduled(cronTaskScanHostsName, nil)
		if err != nil {
			return err
		}

		err = crn.CreateRecurringOneOffJob(cronTaskScanHostsName, nil)
		if err != nil {
			return err
		}
	} else {
		err := crn.CreateExistingJobScheduled(uuid.UUID(scanJobItem.UUID))
		if err != nil {
			return err
		}
	}

	return nil
}

// updateHosts is the cron task handler that delegates to the scanner
func (t *HostManager) updateHosts(_ core.CronTaskArgs, ctx core.Context) error {
	t.ctx.Logger().Info("Starting host scan job")
	return t.scanner.ScanForHosts(ctx)
}
