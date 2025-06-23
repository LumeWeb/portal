package cron

import (
	"context"
	"fmt"
	"github.com/go-co-op/gocron/mocks/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestStandaloneCoordinator_SetHeartbeat(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()

		// Create job in DB first
		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
			JobType: "test-job",
		}
		require.NoError(tb, db.Create(&job).Error)

		// Mock CronService, StateMachineRegistry, StateMachine and JobFactory
		mockCronService := core.GetService[*coreMocks.MockCronService](ctx, core.CRON_SERVICE)
		mockStateMachine := coreMocks.NewMockCronJobStateMachine(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory)
		// Expect a call to Transition on the state machine
		mockStateMachine.EXPECT().Transition(
			mock.Anything, // context.Context
			jobID,
			models.CronJobStateRunning,
			mock.Anything, // options
		).Return(nil).Once()

		// Create StandaloneCoordinator
		coordinator, err := NewStandaloneCoordinator(ctx, mockCronService, NewStateMachineRegistry(ctx), NewCoordinatorOptions().WithStateMachine(mockStateMachine))
		require.NoError(t, err)

		// Execute
		err = coordinator.SetHeartbeat(jobID)
		require.NoError(t, err)

		// Assert that Transition was called
		mockStateMachine.AssertExpectations(t)
	}, coreTesting.WithMockServiceFactory(core.CRON_SERVICE, coreMocks.NewMockCronService))
}

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
		coordinator, err := NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			NewCoordinatorOptions().WithJobCreator(NewJobCreator(db, mockJobFactory, ctx.Logger())),
		)

		// Execute
		alive, err := coordinator.CheckHeartbeat(jobID)
		require.NoError(t, err)
		assert.True(t, alive)

		// Update the job in the database with an old heartbeat
		oldHeartbeat := now.Add(-3 * time.Minute)
		job.LastHeartbeat = &oldHeartbeat
		db.Save(&job)

		// Execute
		alive, err = coordinator.CheckHeartbeat(jobID)
		require.NoError(t, err)
		assert.False(t, alive)

		// Test when job doesn't exist
		nonExistentID := uuid.New()
		alive, err = coordinator.CheckHeartbeat(nonExistentID)
		assert.Error(t, err)
		assert.False(t, alive)
	})
}

func TestStandaloneCoordinator_CreateJobFromDB(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		testJobType := "test-job"

		// Create test record in DB first
		err := db.Create(&models.CronJob{
			UUID:    types.FromUUID(jobID),
			JobType: testJobType,
		}).Error
		require.NoError(tb, err)

		// Mock CronService and JobFactory
		mockCronService := coreMocks.NewMockCronService(t)
		mockJobFactory := coreMocks.NewMockCronJobFactory(t)

		// Expect a call to CreateJob with the test job type
		mockJobFactory.EXPECT().CreateJob(testJobType).Return(coreMocks.NewMockCronJob(t), nil).Once()

		// Create StandaloneCoordinator
		coordinator, err := NewStandaloneCoordinator(
			ctx,
			mockCronService,
			coreMocks.NewMockCronJobStateMachineRegistry(t),
			NewCoordinatorOptions().WithJobCreator(NewJobCreator(db, mockJobFactory, ctx.Logger())),
		)
		require.NoError(t, err)

		// Execute
		job, err := coordinator.CreateJobFromDB(jobID)
		require.NoError(tb, err)
		require.NotNil(tb, job)

		// Assert that CreateJob was called
		mockJobFactory.AssertExpectations(t)
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
			SchedDef: fmt.Sprintf(`{"type": "daily", "at_time": "%s"}`, atTime.Format(time.RFC3339)),
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
		mockJob := gocronmocks.NewMockJob(gomockController)
		mockJob.EXPECT().Context().Return(context.Background()).AnyTimes()

		mockScheduler.EXPECT().Update(
			jobID,
			gomock.Any(), // JobDefinition
			gomock.Any(), // Task
			gomock.Any(), // JobOption
		).Return(mockJob, nil).Times(1)

		coordinator, err := NewStandaloneCoordinator(
			ctx,
			mockCronService,
			mockStateMachineRegistry,
			NewCoordinatorOptions().
				WithScheduler(mockScheduler),
		)
		require.NoError(t, err)

		// Execute
		err = coordinator.EnqueueJob(jobID)
		require.NoError(t, err)

		// Assert that Create was called
		mockScheduleRegistry.AssertExpectations(t)
	})
}
