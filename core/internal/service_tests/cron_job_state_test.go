package service_tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
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

		// Create a real state machine registry and state machine
		registry := service.NewStateMachineRegistry(ctx)
		sm := service.NewCronJobStateMachine(ctx, registry)

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

		// Create a real state machine registry and state machine
		registry := service.NewStateMachineRegistry(ctx)
		sm := service.NewCronJobStateMachine(ctx, registry)

		// Attempt an invalid transition (Queued -> Completed directly)
		err := sm.Transition(context.Background(), jobID, models.CronJobStateCompleted)
		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "invalid transition from queued to completed")
	})
}
