package service_tests

import (
	"fmt"
	"testing"

	gocronmocks "github.com/go-co-op/gocron/mocks/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
	"go.uber.org/mock/gomock"
)

func TestCronServiceDefault_RegisterJob(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Mock the coordinator
		mockCoordinator := coreMocks.NewMockCronCoordinator(t)
		jobID := uuid.New()
		mockJob := coreMocks.NewMockCronJob(t)
		mockCoordinator.EXPECT().EnqueueJob(mock.Anything, jobID).Return(nil).Once()
		mockJob.EXPECT().ID().Return(jobID)
		mockJob.EXPECT().Origin().Return(core.JobOriginCore)
		mockJob.EXPECT().SourceID().Return("test-source")
		mockJob.EXPECT().Type().Return("core.test.job").Maybe()
		mockJob.EXPECT().Args().Return(map[string]interface{}{"test": "value"})
		mockJob.EXPECT().Schedule().Return(&core.CronScheduleDefinition{Type: core.CronScheduleTypeDaily})

		// Create a testing cron service with initialized dependencies
		cronService := service.NewTestingCronService(
			ctx,
			db,
			mockCoordinator,
			service.NewJobFactory(service.NewScheduleRegistry()),
			service.NewScheduleRegistry(),
			service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx)),
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Register the job
		err := cronService.RegisterJob(nil, mockJob, nil)
		require.NoError(t, err)

		// Verify that the job was created in the database
		var job models.CronJob
		result := db.First(&job, "uuid = ?", types.FromUUID(jobID))
		require.NoError(t, result.Error)
		assert.Equal(t, "core.test.job", job.JobType)
		assert.Equal(t, core.JobOriginCore, job.Origin)
		assert.Equal(t, "test-source", job.SourceID)
	})
}

func TestCronServiceDefault_RegisterJob_InvalidOrigin(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a testing cron service with initialized dependencies
		cronService := service.NewTestingCronService(
			ctx,
			db,
			coreMocks.NewMockCronCoordinator(t),
			service.NewJobFactory(service.NewScheduleRegistry()),
			service.NewScheduleRegistry(),
			service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx)),
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Create a mock CronJob
		mockJob := coreMocks.NewMockCronJob(t)
		mockJob.EXPECT().Origin().Return("invalid-origin")

		// Register the job
		err := cronService.RegisterJob(nil, mockJob, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid job origin")
	})
}

func TestCronServiceDefault_RunJob(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Mock the coordinator first
		mockCoordinator := coreMocks.NewMockCronCoordinator(t)
		jobID := uuid.New()
		mockCoordinator.EXPECT().EnqueueJob(mock.Anything, jobID).Return(nil)

		// Create a testing cron service with the coordinator
		cronService := service.NewTestingCronService(
			ctx,
			db,
			mockCoordinator,
			nil,
			nil,
			nil,
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Run the job
		err := cronService.RunJob(nil, jobID)
		require.NoError(t, err)

		// Assert that EnqueueJob was called
		mockCoordinator.AssertExpectations(t)
	})
}

func TestCronServiceDefault_RegisterJobType(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Mock the job factory
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)

		// Expect RegisterFactory to be called with any function of the correct type
		jobType := "test.job"
		var defaultSchedule *core.CronScheduleDefinition
		jobFactory := func() (core.CronJob, error) {
			return nil, nil
		}

		mockJobFactory.EXPECT().RegisterFactory(
			mock.Anything,
			jobType,
			mock.MatchedBy(func(f interface{}) bool {
				_, ok := f.(core.CronJobFactoryFunc)
				return ok
			}),
			defaultSchedule,
		).Return(nil)

		// Create a testing cron service
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			mockJobFactory,
			nil, // Mocked later
			nil, // Mocked later
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Register the job type
		err := cronService.RegisterJobType(nil, jobType, jobFactory, defaultSchedule)
		require.NoError(t, err)

		// Assert that RegisterFactory was called
		mockJobFactory.AssertExpectations(t)
	})
}

func TestCronServiceDefault_ScheduleRegistry(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a testing cron service
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			nil, // Mocked later
			service.NewScheduleRegistry(),
			nil, // Mocked later
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Get the schedule registry
		registry := cronService.ScheduleRegistry()
		assert.NotNil(t, registry)
	})
}

func TestCronServiceDefault_JobFactory(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a testing cron service
		jobFactory := service.NewJobFactory(service.NewScheduleRegistry())
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			jobFactory,
			nil, // Mocked later
			nil, // Mocked later
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Get the job factory
		factory := cronService.JobFactory()
		assert.NotNil(t, factory)
		assert.Equal(t, jobFactory, factory)
	})
}

func TestCronServiceDefault_StateMachine(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a testing cron service
		stateMachine := service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx))
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			nil, // Mocked later
			nil, // Mocked later
			stateMachine,
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Get the state machine
		retrievedStateMachine := cronService.StateMachine()
		assert.NotNil(t, retrievedStateMachine)
		assert.Equal(t, stateMachine, retrievedStateMachine)
	})
}

func TestCronServiceDefault_RegisterPluginJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Mock the job factory
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)

		// Define a plugin info with cron jobs
		pluginInfo := core.PluginInfo{
			ID: "test",
			CronJobs: []core.PluginCronJob{
				{
					Name: "plugin.test.job1",
					Factory: func() (core.CronJob, error) {
						mockJob := coreMocks.NewMockCronJob(t)
						mockJob.EXPECT().ID().Return(uuid.New())
						mockJob.EXPECT().Origin().Return(core.JobOriginPlugin)
						mockJob.EXPECT().SourceID().Return("test")
						mockJob.EXPECT().Type().Return("plugin.test.job1")
						mockJob.EXPECT().Args().Return(map[string]interface{}{})
						mockJob.EXPECT().Schedule().Return(&core.CronScheduleDefinition{
							Type: core.CronScheduleTypeDaily,
						})
						return mockJob, nil
					},
					Schedule: &core.CronScheduleDefinition{
						Type: core.CronScheduleTypeDaily,
					},
				},
				{
					Name: "plugin.test.job2",
					Factory: func() (core.CronJob, error) {
						mockJob := coreMocks.NewMockCronJob(t)
						mockJob.EXPECT().ID().Return(uuid.New())
						mockJob.EXPECT().Origin().Return(core.JobOriginPlugin)
						mockJob.EXPECT().SourceID().Return("test")
						mockJob.EXPECT().Type().Return("plugin.test.job2")
						mockJob.EXPECT().Args().Return(map[string]interface{}{})
						mockJob.EXPECT().Schedule().Return(&core.CronScheduleDefinition{
							Type: core.CronScheduleTypeWeekly,
						})
						return mockJob, nil
					},
					Schedule: &core.CronScheduleDefinition{
						Type: core.CronScheduleTypeWeekly,
					},
				},
			},
		}

		// Expect RegisterFactory to be called for each job
		mockJobFactory.EXPECT().RegisterFactory(
			mock.Anything,
			fmt.Sprintf("plugin.%s.%s", pluginInfo.ID, pluginInfo.CronJobs[0].Name),
			mock.AnythingOfType("core.CronJobFactoryFunc"),
			pluginInfo.CronJobs[0].Schedule,
		).Return(nil)
		mockJobFactory.EXPECT().RegisterFactory(
			mock.Anything,
			fmt.Sprintf("plugin.%s.%s", pluginInfo.ID, pluginInfo.CronJobs[1].Name),
			mock.AnythingOfType("core.CronJobFactoryFunc"),
			pluginInfo.CronJobs[1].Schedule,
		).Return(nil)

		core.RegisterPlugin(pluginInfo)

		// Create a testing cron service with the mock job factory
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			mockJobFactory,
			nil, // Mocked later
			nil, // Mocked later
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Register the plugin jobs
		err := cronService.RegisterPluginJobs(nil, pluginInfo)
		require.NoError(t, err)

		// Assert that RegisterFactory was called for each job
		mockJobFactory.AssertExpectations(t)
	})
}

func TestCronServiceDefault_NewCronService(t *testing.T) {
	cronService, opts, err := service.NewCronService()
	require.NoError(t, err)
	require.NotNil(t, cronService)
	require.NotEmpty(t, opts)
}

func TestCronServiceDefault_StartStop(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Mock the scheduler
		gomockController := gomock.NewController(t)
		mockScheduler := gocronmocks.NewMockScheduler(gomockController)

		// Create a mock state machine registry
		mockStateMachineRegistry := coreMocks.NewMockCronJobStateMachineRegistry(t)

		// Create a testing cron service first (needed by coordinator for JobFactory)
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // coordinator set below
			nil,
			nil,
			nil,
			service.NewDefaultCronMonitor(ctx, nil),
		)

		// Create a coordinator with the mock scheduler
		coordinator, err := service.NewStandaloneCoordinator(ctx, cronService, mockStateMachineRegistry, service.NewCoordinatorOptions().WithScheduler(mockScheduler))
		require.NoError(t, err)

		// Set the coordinator on the cron service
		cronService.(*service.CronServiceDefault).SetCoordinator(coordinator)

		// Expect Start, Update, and Shutdown to be called
		mockScheduler.EXPECT().Start().Return().AnyTimes()
		mockScheduler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockScheduler.EXPECT().Shutdown().Return(nil).AnyTimes()

		// Start the cron service
		err = cronService.Start(nil)
		require.NoError(t, err)

		// Stop the cron service
		err = cronService.Stop(nil)
		require.NoError(t, err)

		// Assert that Start and Shutdown were called
		gomockController.Finish()
	})
}

func TestCronServiceDefault_StartWithCronDisabledSkipsRecoveryAndScheduler(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		_, cfgErr := coreTesting.WithConfig("core.cron.enabled", false)(ctx)
		require.NoError(t, cfgErr)

		db := ctx.DB()
		require.NotNil(tb, db)

		coord := coreMocks.NewMockCronCoordinator(tb)
		monitor := coreMocks.NewMockCronMonitor(tb)

		// Maintenance-job registration still enqueues jobs even when the
		// scheduler is disabled, so allow EnqueueJob/Jobs.
		coord.EXPECT().Jobs().Return(nil).Maybe()
		coord.EXPECT().EnqueueJob(mock.Anything, mock.Anything).Return(nil).Maybe()
		monitor.EXPECT().CleanupOrphanedJobs(mock.Anything).Return(0, nil).Maybe()
		// coordinator.Start() and monitor.RequeueStuckJobs() must NOT be
		// called when cron is disabled. If either is invoked, the testify mock
		// fails the test.

		cronService := service.NewTestingCronService(ctx, db, coord, nil, nil, nil, monitor)
		err := cronService.Start(nil)
		require.NoError(t, err)
	})
}

func TestCronServiceDefault_Monitor(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a testing cron service
		monitor := service.NewDefaultCronMonitor(ctx, nil)
		cronService := service.NewTestingCronService(
			ctx,
			db,
			nil, // Mocked later
			nil, // Mocked later
			nil, // Mocked later
			nil, // Mocked later
			monitor,
		)

		// Get the monitor
		retrievedMonitor := cronService.Monitor()
		assert.NotNil(t, retrievedMonitor)
		assert.Equal(t, monitor, retrievedMonitor)
	})
}
