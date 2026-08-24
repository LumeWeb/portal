package service_tests

import (
	"encoding/json"
	"testing"

	gocronmocks "github.com/go-co-op/gocron/mocks/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// deadJobCheckJobType is the persisted job_type of the core Dead Job Check
// maintenance job. It equals core.GetCronJobIdentifier(JobOriginCore,
// "cron.dead_job_check"); hardcoded here because the internal constant is not
// importable from this package.
const deadJobCheckJobType = "core.cron.dead_job_check"

// startReconcilingCronService builds a CronService backed by a mock gocron
// scheduler and runs Start(), which exercises registerMaintenanceJobs and the
// idempotent maintenance-job reconciliation logic.
func startReconcilingCronService(
	t *testing.T,
	tb coreTesting.TB,
	ctx coreTesting.TestContext,
) *service.CronServiceDefault {
	t.Helper()

	gomockController := gomock.NewController(tb)
	t.Cleanup(gomockController.Finish)
	mockScheduler := gocronmocks.NewMockScheduler(gomockController)

	mockStateMachineRegistry := coreMocks.NewMockCronJobStateMachineRegistry(tb)

	cronService := service.NewTestingCronService(
		ctx,
		ctx.DB(),
		nil,
		nil,
		nil,
		nil,
		service.NewDefaultCronMonitor(ctx, nil),
	)

	coordinator, err := service.NewStandaloneCoordinator(
		ctx,
		cronService,
		mockStateMachineRegistry,
		service.NewCoordinatorOptions().WithScheduler(mockScheduler),
	)
	require.NoError(tb, err)

	cronService.(*service.CronServiceDefault).SetCoordinator(coordinator)

	mockScheduler.EXPECT().Start().Return().AnyTimes()
	mockScheduler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockScheduler.EXPECT().RemoveJob(gomock.Any()).Return(nil).AnyTimes()
	mockScheduler.EXPECT().Jobs().Return(nil).AnyTimes()
	mockScheduler.EXPECT().Shutdown().Return(nil).AnyTimes()

	require.NoError(tb, cronService.Start(nil))

	return cronService.(*service.CronServiceDefault)
}

func requireDeadJobCheckJobs(tb coreTesting.TB, ctx coreTesting.TestContext) []models.CronJob {
	var jobs []models.CronJob
	require.NoError(tb, ctx.DB().Where(&models.CronJob{JobType: deadJobCheckJobType}).Find(&jobs).Error)
	return jobs
}

func TestCronServiceDefault_ReconcileDeadJobCheck_CreatesWhenMissing(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		_, cfgErr := coreTesting.WithConfig("core.cron.dead_job_check_interval_minutes", 30)(ctx)
		require.NoError(tb, cfgErr)

		startReconcilingCronService(t, tb, ctx)

		jobs := requireDeadJobCheckJobs(tb, ctx)
		require.Len(tb, jobs, 1, "a single Dead Job Check job should be created on first boot")

		var sched core.CronScheduleDefinition
		require.NoError(tb, json.Unmarshal(jobs[0].SchedDef, &sched))
		require.Equal(t, core.CronScheduleTypeDuration, sched.Type)
		require.Equal(t, uint(30), sched.Interval)
	})
}

func TestCronServiceDefault_ReconcileDeadJobCheck_UpdatesDriftedScheduleAndDedupes(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		_, cfgErr := coreTesting.WithConfig("core.cron.dead_job_check_interval_minutes", 30)(ctx)
		require.NoError(tb, cfgErr)

		// Simulate a deployment that accumulated duplicate Dead Job Check jobs
		// from earlier boots, the eldest carrying the old daily schedule.
		canonical := uuid.New()
		dup := uuid.New()
		oldSched := datatypes.JSON([]byte(`{"type":"daily"}`))
		for _, j := range []models.CronJob{
			{UUID: types.FromUUID(canonical), JobType: deadJobCheckJobType, SchedDef: oldSched, State: models.CronJobStateQueued},
			{UUID: types.FromUUID(dup), JobType: deadJobCheckJobType, SchedDef: oldSched, State: models.CronJobStateQueued},
		} {
			require.NoError(tb, ctx.DB().Create(&j).Error)
		}

		startReconcilingCronService(t, tb, ctx)

		jobs := requireDeadJobCheckJobs(tb, ctx)
		require.Len(tb, jobs, 1, "duplicate Dead Job Check jobs must be removed")
		require.Equal(t, types.FromUUID(canonical), jobs[0].UUID,
			"the eldest record must be kept as the canonical active job")

		var sched core.CronScheduleDefinition
		require.NoError(tb, json.Unmarshal(jobs[0].SchedDef, &sched))
		require.Equal(t, core.CronScheduleTypeDuration, sched.Type)
		require.Equal(t, uint(30), sched.Interval)
	})
}

func TestCronServiceDefault_ReconcileDeadJobCheck_KeepsMatchingSchedule(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		_, cfgErr := coreTesting.WithConfig("core.cron.dead_job_check_interval_minutes", 30)(ctx)
		require.NoError(tb, cfgErr)

		jobID := uuid.New()
		matching := datatypes.JSON([]byte(`{"type":"duration","interval":30}`))
		require.NoError(tb, ctx.DB().Create(&models.CronJob{
			UUID:    types.FromUUID(jobID),
			JobType: deadJobCheckJobType,
			SchedDef: matching,
			State:   models.CronJobStateQueued,
		}).Error)

		startReconcilingCronService(t, tb, ctx)

		jobs := requireDeadJobCheckJobs(tb, ctx)
		require.Len(tb, jobs, 1, "no duplicate should be created when only one exists")
		require.Equal(t, string(matching), string(jobs[0].SchedDef),
			"a matching schedule must be left untouched")
	})
}
