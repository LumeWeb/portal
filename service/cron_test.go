package service

import (
	"fmt"
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
	"go.lumeweb.com/portal/service/internal/cron"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestCronServiceDefault_RegisterJob(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault with initialized dependencies
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       cron.NewJobFactory(cron.NewScheduleRegistry()),
			scheduleRegistry: cron.NewScheduleRegistry(),
			stateMachine:     cron.NewCronJobStateMachine(ctx, cron.NewStateMachineRegistry(ctx)),
			coordinator:      nil, // Mocked later
		}

		cronService.logger = ctx.ServiceLogger(cronService)

		// Create a mock CronJob
		mockJob := coreMocks.NewMockCronJob(t)
		jobID := uuid.New()
		mockJob.EXPECT().ID().Return(jobID)
		mockJob.EXPECT().Origin().Return(core.JobOriginCore)
		mockJob.EXPECT().SourceID().Return("test-source")
		mockJob.EXPECT().Type().Return("core.test.job").Times(1)
		mockJob.EXPECT().Args().Return(map[string]interface{}{"test": "value"})
		mockJob.EXPECT().Schedule().Return(&core.CronScheduleDefinition{Type: core.CronScheduleTypeDaily})

		// Mock the coordinator
		mockCoordinator := coreMocks.NewMockCronCoordinator(t)
		mockCoordinator.EXPECT().EnqueueJob(jobID).Return(nil).Once()
		cronService.coordinator = mockCoordinator

		// Register the job
		err := cronService.RegisterJob(mockJob)
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

		// Create a mock CronServiceDefault with initialized dependencies
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       cron.NewJobFactory(cron.NewScheduleRegistry()),
			scheduleRegistry: cron.NewScheduleRegistry(),
			stateMachine:     cron.NewCronJobStateMachine(ctx, cron.NewStateMachineRegistry(ctx)),
			coordinator:      coreMocks.NewMockCronCoordinator(t),
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Create a mock CronJob
		mockJob := coreMocks.NewMockCronJob(t)
		mockJob.EXPECT().Origin().Return("invalid-origin")

		// Register the job
		err := cronService.RegisterJob(mockJob)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid job origin")
	})
}

func TestCronServiceDefault_RunJob(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)
		// Mock the coordinator
		mockCoordinator := coreMocks.NewMockCronCoordinator(t)
		jobID := uuid.New()
		mockCoordinator.EXPECT().EnqueueJob(jobID).Return(nil)
		cronService.coordinator = mockCoordinator

		// Run the job
		err := cronService.RunJob(jobID)
		require.NoError(t, err)

		// Assert that EnqueueJob was called
		mockCoordinator.AssertExpectations(t)
	})
}

func TestCronServiceDefault_RegisterJobType(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Mock the job factory
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		cronService.jobFactory = mockJobFactory

		// Define a job type and factory
		jobType := "test.job"
		var defaultSchedule *core.CronScheduleDefinition
		jobFactory := func() (core.CronJob, error) {
			return nil, nil
		}

		// Expect RegisterFactory to be called with any function of the correct type
		mockJobFactory.EXPECT().RegisterFactory(
			jobType,
			mock.MatchedBy(func(f interface{}) bool {
				_, ok := f.(core.CronJobFactoryFunc)
				return ok
			}),
			defaultSchedule,
		).Return(nil)

		// Register the job type
		err := cronService.RegisterJobType(jobType, jobFactory, defaultSchedule)
		require.NoError(t, err)

		// Assert that RegisterFactory was called
		mockJobFactory.AssertExpectations(t)
	})
}

func TestCronServiceDefault_ScheduleRegistry(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: cron.NewScheduleRegistry(),
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Get the schedule registry
		registry := cronService.ScheduleRegistry()
		assert.NotNil(t, registry)
	})
}

func TestCronServiceDefault_JobFactory(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx: ctx,
			db:  db,

			jobFactory:       cron.NewJobFactory(cron.NewScheduleRegistry()),
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Get the job factory
		factory := cronService.JobFactory()
		assert.NotNil(t, factory)
	})
}

func TestCronServiceDefault_StateMachine(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     cron.NewCronJobStateMachine(ctx, cron.NewStateMachineRegistry(ctx)),
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Get the state machine
		stateMachine := cronService.StateMachine()
		assert.NotNil(t, stateMachine)
	})
}

func TestCronServiceDefault_RegisterPluginJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Mock the job factory
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		cronService.jobFactory = mockJobFactory

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
			fmt.Sprintf("plugin.%s.%s", pluginInfo.ID, pluginInfo.CronJobs[0].Name),
			mock.AnythingOfType("core.CronJobFactoryFunc"),
			pluginInfo.CronJobs[0].Schedule,
		).Return(nil)
		mockJobFactory.EXPECT().RegisterFactory(
			fmt.Sprintf("plugin.%s.%s", pluginInfo.ID, pluginInfo.CronJobs[1].Name),
			mock.AnythingOfType("core.CronJobFactoryFunc"),
			pluginInfo.CronJobs[1].Schedule,
		).Return(nil)

		core.RegisterPlugin(pluginInfo)

		// Register the plugin jobs
		err := cronService.RegisterPluginJobs(pluginInfo)
		require.NoError(t, err)

		// Assert that RegisterFactory was called for each job
		mockJobFactory.AssertExpectations(t)
	})
}

func TestCronServiceDefault_NewCronService(t *testing.T) {
	cronService, opts, err := NewCronService()
	require.NoError(t, err)
	require.NotNil(t, cronService)
	require.NotEmpty(t, opts)
}

func TestCronServiceDefault_StartStop(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx:              ctx,
			db:               db,
			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Mock the scheduler
		gomockController := gomock.NewController(t)
		mockScheduler := gocronmocks.NewMockScheduler(gomockController)
		coordinator, err := cron.NewStandaloneCoordinator(ctx, cronService, coreMocks.NewMockCronJobStateMachineRegistry(t), cron.NewCoordinatorOptions().WithScheduler(mockScheduler))
		require.NoError(t, err)
		cronService.coordinator = coordinator

		// Expect Start and Shutdown to be called
		mockScheduler.EXPECT().Start().Return().Times(1)
		mockScheduler.EXPECT().Shutdown().Return(nil).Times(1)

		// Start the cron service
		err = cronService.Start()
		require.NoError(t, err)

		// Stop the cron service
		err = cronService.Stop()
		require.NoError(t, err)

		// Assert that Start and Shutdown were called
		gomockController.Finish()
	})
}

func TestCronServiceDefault_Monitor(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronServiceDefault
		cronService := &CronServiceDefault{
			ctx: ctx,
			db:  db,

			jobFactory:       nil, // Mocked later
			scheduleRegistry: nil, // Mocked later
			stateMachine:     nil, // Mocked later
			coordinator:      nil, // Mocked later
			monitor:          cron.NewDefaultCronMonitor(ctx, nil),
		}
		cronService.logger = ctx.ServiceLogger(cronService)

		// Get the monitor
		monitor := cronService.Monitor()
		assert.NotNil(t, monitor)
	})
}
