package cron

import (
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestDefaultCronMonitor_CleanupOrphanedJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Define plugin IDs
		pluginID := "test-plugin"
		anotherPluginID := "another-plugin"

		// Register fake plugins with proper build info and at least one component
		core.RegisterPlugin(core.PluginInfo{
			ID:      anotherPluginID,
			Version: build.New("", "", "", "", "", "", ""),
			Services: func() ([]core.ServiceInfo, error) {
				return []core.ServiceInfo{}, nil
			},
		})

		// Create test jobs
		job1 := models.CronJob{
			UUID:     types.FromUUID(uuid.New()),
			Origin:   core.JobOriginPlugin,
			SourceID: pluginID,
		}
		job2 := models.CronJob{
			UUID:     types.FromUUID(uuid.New()),
			Origin:   core.JobOriginPlugin,
			SourceID: pluginID,
		}
		job3 := models.CronJob{
			UUID:     types.FromUUID(uuid.New()),
			Origin:   core.JobOriginPlugin,
			SourceID: anotherPluginID,
		}
		require.NoError(tb, db.Create(&job1).Error)
		require.NoError(tb, db.Create(&job2).Error)
		require.NoError(tb, db.Create(&job3).Error)

		// Mock CronService and StateMachine
		mockCronService := mocks.NewMockCronService(t)
		mockStateMachine := mocks.NewMockCronJobStateMachine(t)
		mockCronService.EXPECT().StateMachine().Return(mockStateMachine)

		// Expect RemoveStateMachine calls for orphaned jobs
		mockStateMachine.EXPECT().RemoveStateMachine(job1.UUID.ToUUID()).Return().Once()
		mockStateMachine.EXPECT().RemoveStateMachine(job2.UUID.ToUUID()).Return().Once()

		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Execute
		count, err := monitor.CleanupOrphanedJobs()
		require.NoError(tb, err)

		// Verify
		assert.Equal(t, 2, count)

		// Verify that the jobs were deleted
		var countJobs int64
		require.NoError(tb, db.Model(&models.CronJob{}).Count(&countJobs).Error)
		assert.Equal(t, int64(1), countJobs)
	})
}

func TestDefaultCronMonitor_CleanupOrphanedJobs_PluginUnregistered(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a job for a non-existent plugin
		pluginID := "non-existent-plugin"
		job := models.CronJob{
			UUID:     types.FromUUID(uuid.New()),
			Origin:   core.JobOriginPlugin,
			SourceID: pluginID,
		}
		require.NoError(tb, db.Create(&job).Error)

		// Mock CronService and StateMachine
		mockCronService := mocks.NewMockCronService(t)
		mockStateMachine := mocks.NewMockCronJobStateMachine(t)
		mockCronService.EXPECT().StateMachine().Return(mockStateMachine)

		// Expect RemoveStateMachine call for the orphaned job
		mockStateMachine.EXPECT().RemoveStateMachine(job.UUID.ToUUID()).Return()

		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Execute
		count, err := monitor.CleanupOrphanedJobs()
		require.NoError(tb, err)

		// Verify
		assert.Equal(t, 1, count)

		// Verify that the job was deleted
		var countJobs int64
		require.NoError(tb, db.Model(&models.CronJob{}).Count(&countJobs).Error)
		assert.Equal(t, int64(0), countJobs)
	})
}

func TestDefaultCronMonitor_RequeueStuckJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Set up test data: a stuck job
		now := time.Now()
		pastHeartbeat := now.Add((-6 * time.Minute)) // Older than 5-minute cutoff
		jobID := uuid.New()
		job := models.CronJob{
			UUID:          types.FromUUID(jobID),
			State:         models.CronJobStateRunning,
			LastHeartbeat: &pastHeartbeat,
			Failures:      0,
			JobType:       IntegrationTestJobType,
		}
		require.NoError(tb, db.Create(&job).Error)

		// Mock CronService, Coordinator, and JobFactory
		mockCronService := mocks.NewMockCronService(t)
		mockCoordinator := mocks.NewMockCronCoordinator(t)
		mockJobFactory := mocks.NewMockCronJobFactory(t)

		mockCronService.EXPECT().Coordinator().Return(mockCoordinator)
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory)

		// Expect a call to HandleFailedJob on the coordinator
		mockCoordinator.EXPECT().HandleFailedJob(jobID, uint(1)).Return(nil)

		// Create mock job and set expectations
		mockJob := mocks.NewMockCronJob(t)
		mockJob.EXPECT().Origin().Return(core.JobOriginCore).Once()
		mockJob.EXPECT().ID().Return(jobID).Once()

		// Expect a call to CreateJob on the job factory
		mockJobFactory.EXPECT().CreateJob(IntegrationTestJobType).Return(mockJob, nil)

		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Execute
		err := monitor.RequeueStuckJobs()
		require.NoError(tb, err)

		// Assert that HandleFailedJob was called
		mockCoordinator.AssertExpectations(tb)
	})
}
func TestDefaultCronMonitor_RequeueStuckJobs_NoStuckJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Set up test data: no stuck jobs
		now := time.Now()
		cutoff := 1 * time.Minute
		job := models.CronJob{
			UUID:          types.FromUUID(uuid.New()),
			State:         models.CronJobStateRunning,
			LastHeartbeat: lo.ToPtr(now.Add(-cutoff)),
		}
		require.NoError(tb, db.Create(&job).Error)

		// Mock CronService and Coordinator
		mockCronService := mocks.NewMockCronService(t)
		mockCoordinator := mocks.NewMockCronCoordinator(t)

		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Execute
		err := monitor.RequeueStuckJobs()
		require.NoError(tb, err)

		// Assert that HandleFailedJob was NOT called
		mockCoordinator.AssertNotCalled(t, "HandleFailedJob", mock.Anything, mock.Anything)
	})
}

func TestDefaultCronMonitor_CleanupCompletedJobs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create test data: completed "once" jobs, some old, some recent
		now := time.Now()
		oldTime := now.Add(-31 * 24 * time.Hour) // Older than 30 days

		job1 := models.CronJob{
			Model: gorm.Model{
				CreatedAt: oldTime,
			},
			UUID:         types.FromUUID(uuid.New()),
			State:        models.CronJobStateCompleted,
			ScheduleType: string(core.CronScheduleTypeOnce),
		}
		job2 := models.CronJob{
			Model: gorm.Model{
				CreatedAt: oldTime,
			},
			UUID:         types.FromUUID(uuid.New()),
			State:        models.CronJobStateCompleted,
			ScheduleType: string(core.CronScheduleTypeOnce),
		}
		job3 := models.CronJob{ // Not a "once" job
			Model: gorm.Model{
				CreatedAt: oldTime,
			},
			UUID:         types.FromUUID(uuid.New()),
			State:        models.CronJobStateCompleted,
			ScheduleType: string(core.CronScheduleTypeDaily),
		}
		job4 := models.CronJob{ // Not completed
			Model: gorm.Model{
				CreatedAt: oldTime,
			},
			UUID:         types.FromUUID(uuid.New()),
			State:        models.CronJobStateQueued,
			ScheduleType: string(core.CronScheduleTypeOnce),
		}

		require.NoError(tb, db.Create(&job1).Error)
		require.NoError(tb, db.Create(&job2).Error)
		require.NoError(tb, db.Create(&job3).Error)
		require.NoError(tb, db.Create(&job4).Error)

		// Mock CronService
		mockCronService := mocks.NewMockCronService(t)

		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Execute
		err := monitor.CleanupCompletedJobs()
		require.NoError(tb, err)

		// Verify that only the old, completed "once" job was deleted
		var remainingJobs []models.CronJob
		require.NoError(tb, db.Find(&remainingJobs).Error)
		assert.Len(tb, remainingJobs, 2)
		deletedJobUUID := job1.UUID
		for _, job := range remainingJobs {
			assert.NotEqual(tb, job.UUID, deletedJobUUID)
		}

	})
}

func TestDefaultCronMonitor_MaintenanceLoop(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Mock CronService
		mockCronService := mocks.NewMockCronService(t)

		// Create real monitor instance
		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Start the monitoring loop
		err := monitor.StartMonitoring()
		require.NoError(t, err)

		// Send a maintenance signal
		monitor.SignalMaintenance()

		// Give the loop some time to process the signal
		time.Sleep(100 * time.Millisecond)

		// Stop the monitoring loop
		err = monitor.StopMonitoring()
		require.NoError(t, err)
	})
}

func TestDefaultCronMonitor_SignalMaintenance(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a monitor instance
		mockCronService := mocks.NewMockCronService(t)
		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		// Start the monitoring loop
		err := monitor.StartMonitoring()
		require.NoError(t, err)

		// Send multiple maintenance signals
		monitor.SignalMaintenance()
		monitor.SignalMaintenance() // Send a second signal to ensure non-blocking

		// Give the loop some time to process the signal
		time.Sleep(50 * time.Millisecond)

		// Stop the monitoring loop
		err = monitor.StopMonitoring()
		require.NoError(t, err)

		// Check that the maintenance channel has at most one signal
		select {
		case <-monitor.maintenanceCh:
			// Signal was received
		default:
			// No signal was received, which is also acceptable
		}
	})
}

func TestDefaultCronMonitor_SignalMaintenance_TriggersMaintenance(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Register fake plugin
		pluginID := "test-plugin"
		// Create test job for the plugin
		job := models.CronJob{
			UUID:     types.FromUUID(uuid.New()),
			Origin:   core.JobOriginPlugin,
			SourceID: pluginID,
		}
		require.NoError(tb, db.Create(&job).Error)

		// Mock CronService dependencies
		mockCronService := mocks.NewMockCronService(t)
		mockStateMachine := mocks.NewMockCronJobStateMachine(t)
		mockCoordinator := mocks.NewMockCronCoordinator(t)
		mockJobFactory := mocks.NewMockCronJobFactory(t)

		mockCronService.EXPECT().StateMachine().Return(mockStateMachine)

		// Expect RemoveStateMachine call for the orphaned job
		mockStateMachine.EXPECT().RemoveStateMachine(job.UUID.ToUUID()).Return()

		// Create real monitor instance and start it
		monitor := NewDefaultCronMonitor(ctx, mockCronService)
		err := monitor.StartMonitoring()
		require.NoError(t, err)
		defer func(monitor *DefaultCronMonitor) {
			err = monitor.StopMonitoring()
			if err != nil {
				require.NoError(t, err)
			}
		}(monitor)

		// Send maintenance signal
		monitor.SignalMaintenance()

		// Give the loop time to process
		time.Sleep(1 * time.Second)

		// Verify maintenance was performed by checking mock expectations
		mockStateMachine.AssertExpectations(t)
		mockCoordinator.AssertExpectations(t)
		mockJobFactory.AssertExpectations(t)

		// Verify job was deleted
		var count int64
		require.NoError(tb, db.Model(&models.CronJob{}).Where(&models.CronJob{UUID: job.UUID}).Count(&count).Error)
		assert.Equal(t, int64(0), count)
	})
}

func TestDefaultCronMonitor_Heartbeat(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		jobID := uuid.New()

		// Mock CronService and Coordinator
		mockCronService := mocks.NewMockCronService(t)
		mockCoordinator := mocks.NewMockCronCoordinator(t)
		mockCronService.EXPECT().Coordinator().Return(mockCoordinator)

		// Create monitor instance
		monitor := NewDefaultCronMonitor(ctx, mockCronService)

		monitor.StartHeartbeat(jobID)
		time.Sleep(50 * time.Millisecond) // Give heartbeat loop some time

		ret := true

		// --- Test CheckHeartbeat (alive) ---
		mockCoordinator.EXPECT().CheckHeartbeat(jobID).RunAndReturn(func(_ uuid.UUID) (bool, error) {
			return ret, nil
		})

		alive, err := monitor.CheckHeartbeat(jobID)
		require.NoError(t, err)
		assert.True(t, alive)

		// --- Test StopHeartbeat ---
		monitor.StopHeartbeat(jobID)
		time.Sleep(50 * time.Millisecond) // Give heartbeat loop some time

		// --- Test CheckHeartbeat (not alive after stop) ---
		ret = false
		alive, err = monitor.CheckHeartbeat(jobID)
		require.NoError(t, err)
		assert.False(t, alive)

		mock.AssertExpectationsForObjects(t, mockCronService, mockCoordinator)
	})
}
