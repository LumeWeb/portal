package cron

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/looplab/fsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
)

func TestStateMachineRegistry_GetOrCreate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		registry := NewStateMachineRegistry(ctx)

		// Test creating new machine
		job, machine, err := registry.GetOrCreate(ctx, jobID)
		assert.Error(t, err) // Should error since job doesn't exist yet
		assert.Nil(t, job)
		assert.Nil(t, machine)

		// Create job in DB
		job = &models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		require.NoError(t, db.Create(job).Error)

		// Test getting existing machine
		job, machine, err = registry.GetOrCreate(ctx, jobID)
		assert.NoError(t, err)
		assert.NotNil(t, job)
		assert.NotNil(t, machine)
		assert.Equal(t, string(models.CronJobStateQueued), machine.Current())
	})
}

func TestStateMachineRegistry_Transition(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		registry := NewStateMachineRegistry(ctx)

		// Create job in DB
		job := &models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		require.NoError(t, db.Create(job).Error)

		// Get state machine
		_, _fsm, err := registry.GetOrCreate(ctx, jobID)
		require.NoError(t, err)

		// Transition to running state with last run option
		fsmCtx := context.WithValue(ctx.GetContext(), stateMachineDataKey, &stateMachineData{
			jobID:          jobID,
			currentVersion: job.Version,
			params:         &core.CronStateParams{},
		})
		err = _fsm.Event(fsmCtx, stateToEvent[models.CronJobStateRunning], core.WithCronLastRun())
		assert.NoError(t, err)

		// Verify state was persisted
		var updatedJob models.CronJob
		require.NoError(t, db.First(&updatedJob, "uuid = ?", types.FromUUID(jobID)).Error)
		assert.Equal(t, models.CronJobStateRunning, updatedJob.State)
		assert.NotZero(t, updatedJob.LastRun)
		assert.NotZero(t, updatedJob.LastHeartbeat)
		assert.Equal(t, int64(2), updatedJob.Version)
	})
}

func TestStateMachineRegistry_VersionConflict(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		registry := NewStateMachineRegistry(ctx)

		// Create job in DB
		job := &models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		require.NoError(t, db.Create(job).Error)

		// Simulate concurrent update
		require.NoError(t, db.Model(&models.CronJob{}).
			Where("uuid = ?", types.FromUUID(jobID)).
			Update("version", 2).Error)

		// Try to update with old version
		err := registry.persistState(context.Background(), jobID, 1, models.CronJobStateRunning, &core.CronStateParams{})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrVersionMismatch))
	})
}

func TestStateMachineRegistry_Remove(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		registry := NewStateMachineRegistry(ctx)
		jobID := uuid.New()

		// Add dummy machine to registry
		registry.machines[jobID] = fsm.NewFSM(
			string(models.CronJobStateQueued),
			nil,
			nil,
		)

		// Test removal
		registry.Remove(nil, jobID)
		_, exists := registry.machines[jobID]
		assert.False(t, exists)
	})
}
