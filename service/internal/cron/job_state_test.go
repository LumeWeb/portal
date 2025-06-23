package cron

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/looplab/fsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
)

func TestJobStateMachineTransition(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()

		// Create a job in the database
		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		result := db.Create(&job)
		require.NoError(tb, result.Error)

		// Create a mock CronJobStateMachineRegistry
		mockRegistry := NewStateMachineRegistry(ctx)

		sm := NewCronJobStateMachine(ctx, mockRegistry)

		// Test initial state
		assert.Equal(tb, string(models.CronJobStateQueued), sm.State())

		// Test valid transition: Queued -> Running
		err := sm.Transition(context.Background(), jobID, models.CronJobStateRunning)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateRunning), sm.State())

		// Test valid transition: Running -> Completed
		err = sm.Transition(context.Background(), jobID, models.CronJobStateCompleted)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateCompleted), sm.State())

		// Test valid transition: Completed -> Queued
		err = sm.Transition(context.Background(), jobID, models.CronJobStateQueued)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateQueued), sm.State())

		// Test valid transition: Queued -> Running
		err = sm.Transition(context.Background(), jobID, models.CronJobStateRunning)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateRunning), sm.State())

		// Test valid transition: Running -> Failed
		err = sm.Transition(context.Background(), jobID, models.CronJobStateFailed)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateFailed), sm.State())

		// Test valid transition: Failed -> Queued
		err = sm.Transition(context.Background(), jobID, models.CronJobStateQueued)
		require.NoError(tb, err)
		assert.Equal(tb, string(models.CronJobStateQueued), sm.State())
	})
}

func TestJobStateMachineTransition_InvalidState(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()

		// Create a job in the database
		job := models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		result := db.Create(&job)
		require.NoError(tb, result.Error)

		// Create a mock CronJobStateMachineRegistry
		mockRegistry := mocks.NewMockCronJobStateMachineRegistry(tb)

		// Create a mock FSM
		fsmMock := fsm.NewFSM(
			string(models.CronJobStateQueued),
			fsm.Events{},
			fsm.Callbacks{},
		)

		// Set up expectations for GetOrCreate to return the mock FSM
		mockRegistry.EXPECT().GetOrCreate(jobID).Return(&job, fsmMock, nil)

		sm := NewCronJobStateMachine(ctx, mockRegistry)

		// Attempt an invalid transition (e.g., queued -> completed directly)
		err := sm.Transition(context.Background(), jobID, models.CronJobStateCompleted)
		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "invalid transition from queued to completed")
	})
}

func TestNewJobStateMachine(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a mock CronJobStateMachineRegistry
		mockRegistry := mocks.NewMockCronJobStateMachineRegistry(tb)

		sm := NewCronJobStateMachine(ctx, mockRegistry)
		assert.NotNil(tb, sm)
		assert.Equal(tb, string(models.CronJobStateQueued), sm.State())
	})
}

func TestCronJobStateOptions(t *testing.T) {
	t.Run("WithLastRun", func(t *testing.T) {
		params := &core.CronStateParams{}
		core.WithCronLastRun()(params)
		assert.True(t, params.LastRun())
	})

	t.Run("WithFailures", func(t *testing.T) {
		params := &core.CronStateParams{}
		core.WithCronFailures(5)(params)
		assert.Equal(t, 5, params.Failures())
	})

	t.Run("WithHeartbeat", func(t *testing.T) {
		params := &core.CronStateParams{}
		core.WithCronHeartbeat()(params)
		assert.True(t, params.Heartbeat())
	})
}
