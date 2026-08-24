package service_tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-co-op/gocron/mocks/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

func TestStandaloneCoordinator_CheckHeartbeat(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(t, db)

		jobID := uuid.New()
		now := time.Now()
		pastHeartbeat := now.Add(-1 * time.Minute)

		// Create a job in the database with a recent heartbeat
		job := models.CronJob{
			UUID:          types.FromUUID(jobID),
			State:         models.CronJobStateRunning,
			LastHeartbeat: &pastHeartbeat,
		}
		result := db.Create(&job)
		require.NoError(t, result.Error)

		// Mock CronService and JobFactory
		mockCronService := coreMocks.NewMockCronService(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)

		// Create StandaloneCoordinator
		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			service.NewCoordinatorOptions().WithJobCreator(service.NewJobCreator(db, mockJobFactory, ctx.Logger())),
		)
		require.NoError(t, err)

		// Execute — recent heartbeat should be alive
		alive, err := coordinator.CheckHeartbeat(nil, jobID)
		require.NoError(t, err)
		assert.True(t, alive)

		// Update the job in the database with an old heartbeat
		oldHeartbeat := now.Add(-3 * time.Minute)
		job.LastHeartbeat = &oldHeartbeat
		db.Save(&job)

		// Execute — old heartbeat should be dead
		alive, err = coordinator.CheckHeartbeat(nil, jobID)
		require.NoError(t, err)
		assert.False(t, alive)

		// Test when job doesn't exist
		nonExistentID := uuid.New()
		alive, err = coordinator.CheckHeartbeat(nil, nonExistentID)
		assert.Error(t, err)
		assert.False(t, alive)
	})
}

func TestStandaloneCoordinator_SetHeartbeat_BypassesFSM(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(t, db)

		jobID := uuid.New()
		beforeSet := time.Now().Add(-1 * time.Minute)

		// Create a Running job with a stale heartbeat and known version
		job := models.CronJob{
			UUID:          types.FromUUID(jobID),
			State:         models.CronJobStateRunning,
			Version:       3,
			LastHeartbeat: &beforeSet,
		}
		result := db.Create(&job)
		require.NoError(t, result.Error)

		// Create coordinator — the FSM must NOT be called.
		// No mock state machine is wired; if SetHeartbeat goes through the FSM
		// path, it would nil-dereference or call an unmocked method.
		mockCronService := coreMocks.NewMockCronService(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory).Maybe()

		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			service.NewCoordinatorOptions(),
		)
		require.NoError(t, err)

		// Execute
		err = coordinator.SetHeartbeat(nil, jobID)
		require.NoError(t, err)

		// Reload from DB and assert:
		// 1. last_heartbeat was updated
		// 2. state was NOT changed (still Running, not re-transitioned)
		// 3. version was NOT bumped (FSM was bypassed)
		var updated models.CronJob
		result = db.First(&updated, "uuid = ?", types.FromUUID(jobID))
		require.NoError(t, result.Error)

		assert.Equal(t, models.CronJobStateRunning, updated.State,
			"state must not change on heartbeat")
		assert.Equal(t, int64(3), updated.Version,
			"version must not be bumped on heartbeat")
		assert.NotNil(t, updated.LastHeartbeat,
			"last_heartbeat must be set")
		assert.True(t, updated.LastHeartbeat.After(beforeSet),
			"last_heartbeat must be updated to a more recent time")
	})
}

func TestStandaloneCoordinator_EnqueueJob(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(t, db)

		jobID := uuid.New()

		// Create a job in the database
		now := time.Now()
		atTime := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
		job := models.CronJob{
			UUID:     types.FromUUID(jobID),
			State:    models.CronJobStateQueued,
			Version:  1,
			JobType:  "test-job",
			SchedDef: datatypes.JSON(fmt.Sprintf(`{"type": "daily", "at_time": "%s"}`, atTime.Format(time.RFC3339))),
		}
		result := db.Create(&job)
		require.NoError(t, result.Error)

		// Mock CronService and dependencies
		mockCronService := coreMocks.NewMockCronService(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockScheduleRegistry := coreMocks.NewMockCronScheduleRegistry(t)
		mockStateMachineRegistry := coreMocks.NewMockCronJobStateMachineRegistry(t)

		// Set up mock expectations
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory).Maybe()
		mockCronService.EXPECT().ScheduleRegistry().Return(mockScheduleRegistry).Maybe()
		mockScheduleRegistry.EXPECT().Create(mock.Anything).Return(gocron.DurationJob(time.Second), nil).Once()

		// Create coordinator with mock scheduler via options
		gomockController := gomock.NewController(t)
		mockScheduler := gocronmocks.NewMockScheduler(gomockController)
		mockGocronJob := gocronmocks.NewMockJob(gomockController)

		mockScheduler.EXPECT().Update(
			jobID,
			gomock.Any(), // JobDefinition
			gomock.Any(), // Task
			gomock.Any(), // JobOption
		).Return(mockGocronJob, nil).Times(1)

		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			mockStateMachineRegistry,
			service.NewCoordinatorOptions().
				WithScheduler(mockScheduler),
		)
		require.NoError(t, err)

		// Execute
		err = coordinator.EnqueueJob(nil, jobID)
		require.NoError(t, err)

		// Assert that Create was called
		mockScheduleRegistry.AssertExpectations(t)
	})
}

func TestStandaloneCoordinator_HandleFailedJob_ResetsToQueuedOnRetry(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()

		// A Running job with no retry policy. maxFailures defaults to 5, so
		// a failure count of 1 is NOT permanent and should be requeued.
		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateRunning,
			Version: 1,
			JobType: "test-job",
		}
		require.NoError(tb, db.Create(&job).Error)

		// Real state machine backed by the real DB so the full
		// Running -> Failed -> Queued lifecycle is exercised.
		realSM := service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx))

		mockCronService := coreMocks.NewMockCronService(t)
		mockMonitor := coreMocks.NewMockCronMonitor(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockScheduleRegistry := coreMocks.NewMockCronScheduleRegistry(t)

		mockCronService.EXPECT().Monitor().Return(mockMonitor)
		mockMonitor.EXPECT().StopHeartbeat(mock.Anything, jobID).Return().Once()
		mockCronService.EXPECT().StateMachine().Return(realSM).Times(2) // Failed, then Queued
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory).Maybe()
		mockCronService.EXPECT().ScheduleRegistry().Return(mockScheduleRegistry).Maybe()
		mockScheduleRegistry.EXPECT().Create(mock.Anything).Return(gocron.DurationJob(time.Second), nil).Once()

		// Mock the gocron scheduler so EnqueueJob doesn't really schedule.
		gomockController := gomock.NewController(t)
		mockScheduler := gocronmocks.NewMockScheduler(gomockController)
		mockGocronJob := gocronmocks.NewMockJob(gomockController)
		mockScheduler.EXPECT().Update(
			jobID,
			gomock.Any(), // JobDefinition
			gomock.Any(), // Task
			gomock.Any(), // JobOption
		).Return(mockGocronJob, nil).Times(1)

		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			service.NewCoordinatorOptions().
				WithScheduler(mockScheduler),
		)
		require.NoError(t, err)

		// Execute — non-permanent failure, should requeue and reset to Queued
		err = coordinator.HandleFailedJob(nil, jobID, 1)
		require.NoError(t, err)

		// The requeued job must be back in Queued so SetupJob/CleanupJob can
		// cycle it through Running -> Completed on the next execution.
		var updated models.CronJob
		require.NoError(tb, db.First(&updated, "uuid = ?", types.FromUUID(jobID)).Error)
		assert.Equal(t, models.CronJobStateQueued, updated.State,
			"non-permanent retry must reset the job to Queued for clean lifecycle cycling")

		// The accumulated failure count must be preserved through the
		// Failed -> Queued transition. RequeueStuckJobs uses the DB failures
		// column as its source of truth, so zeroing it here would let
		// dead-job-detected retries loop forever without ever hitting the
		// permanent-failure threshold.
		assert.Equal(t, uint(1), updated.Failures,
			"retry must preserve the accumulated failure count")
	})
}

func TestStandaloneCoordinator_HandleFailedJob_RequeueFailureKeepsFailed(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateRunning,
			Version: 1,
			JobType: "test-job",
		}
		require.NoError(tb, db.Create(&job).Error)

		realSM := service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx))

		mockCronService := coreMocks.NewMockCronService(t)
		mockMonitor := coreMocks.NewMockCronMonitor(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockScheduleRegistry := coreMocks.NewMockCronScheduleRegistry(t)

		mockCronService.EXPECT().Monitor().Return(mockMonitor)
		mockMonitor.EXPECT().StopHeartbeat(mock.Anything, jobID).Return().Once()
		// Requeue happens before the Failed -> Queued transition, so if
		// EnqueueJob fails only the Running -> Failed transition occurs.
		mockCronService.EXPECT().StateMachine().Return(realSM).Once()
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory).Maybe()
		mockCronService.EXPECT().ScheduleRegistry().Return(mockScheduleRegistry).Maybe()
		mockScheduleRegistry.EXPECT().Create(mock.Anything).Return(gocron.DurationJob(time.Second), nil).Once()

		// Simulate a scheduler failure during requeue.
		gomockController := gomock.NewController(t)
		mockScheduler := gocronmocks.NewMockScheduler(gomockController)
		mockScheduler.EXPECT().Update(
			jobID,
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).Return(nil, fmt.Errorf("scheduler unavailable")).Times(1)

		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			service.NewCoordinatorOptions().WithScheduler(mockScheduler),
		)
		require.NoError(t, err)

		err = coordinator.HandleFailedJob(nil, jobID, 1)
		require.Error(t, err)

		// Requeue failed, so the job must NOT have been flipped to Queued
		// without being registered with the scheduler. It stays Failed and can
		// be retried later.
		var updated models.CronJob
		require.NoError(tb, db.First(&updated, "uuid = ?", types.FromUUID(jobID)).Error)
		assert.Equal(t, models.CronJobStateFailed, updated.State,
			"job must remain Failed when requeue fails, not be left stuck in Queued")
	})
}

func TestStandaloneCoordinator_HandleFailedJob_PermanentFailureStaysFailed(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()

		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateRunning,
			Version: 1,
			JobType: "test-job",
		}
		require.NoError(tb, db.Create(&job).Error)

		realSM := service.NewCronJobStateMachine(ctx, service.NewStateMachineRegistry(ctx))

		mockCronService := coreMocks.NewMockCronService(t)
		mockMonitor := coreMocks.NewMockCronMonitor(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockCronService.EXPECT().Monitor().Return(mockMonitor)
		mockMonitor.EXPECT().StopHeartbeat(mock.Anything, jobID).Return().Once()
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory).Maybe()
		// Only one transition: Running -> Failed. No requeue happens.
		mockCronService.EXPECT().StateMachine().Return(realSM).Once()

		coordinator, err := service.NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			service.NewCoordinatorOptions(),
		)
		require.NoError(t, err)

		// failures (5) >= maxFailures (5 default) -> permanent
		err = coordinator.HandleFailedJob(nil, jobID, 5)
		require.NoError(t, err)

		var updated models.CronJob
		require.NoError(tb, db.First(&updated, "uuid = ?", types.FromUUID(jobID)).Error)
		assert.Equal(t, models.CronJobStateFailed, updated.State,
			"permanent failure must leave the job in Failed state")
	})
}
