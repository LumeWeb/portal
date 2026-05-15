package service_tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
)

func TestStateMachineRegistry_GetOrCreate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		registry := service.NewStateMachineRegistry(ctx)

		// Test creating new machine — should error since job doesn't exist yet
		job, machine, err := registry.GetOrCreate(ctx, jobID)
		assert.Error(t, err)
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

func TestStateMachineRegistry_TransitionViaPublicAPI(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		jobID := uuid.New()
		registry := service.NewStateMachineRegistry(ctx)
		sm := service.NewCronJobStateMachine(ctx, registry)

		// Create job in DB
		job := &models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		require.NoError(t, db.Create(job).Error)

		// Transition to running state through the public API
		err := sm.Transition(context.Background(), jobID, models.CronJobStateRunning, core.WithCronLastRun())
		require.NoError(t, err)

		// Verify state was persisted
		var updatedJob models.CronJob
		require.NoError(t, db.First(&updatedJob, "uuid = ?", types.FromUUID(jobID)).Error)
		assert.Equal(t, models.CronJobStateRunning, updatedJob.State)
		assert.NotZero(t, updatedJob.LastRun)
		assert.NotZero(t, updatedJob.LastHeartbeat)
		assert.Equal(t, int64(2), updatedJob.Version)
	})
}

func TestStateMachineRegistry_Remove(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		jobID := uuid.New()
		registry := service.NewStateMachineRegistry(ctx)

		// Create job in DB so GetOrCreate works
		db := ctx.DB()
		require.NotNil(tb, db)
		job := &models.CronJob{
			UUID:    types.FromUUID(jobID),
			State:   models.CronJobStateQueued,
			Version: 1,
		}
		require.NoError(t, db.Create(job).Error)

		// Get a machine so it exists in the registry
		_, machine, err := registry.GetOrCreate(ctx, jobID)
		require.NoError(t, err)
		require.NotNil(t, machine)

		// Remove it
		registry.Remove(ctx, jobID)

		// Verify it was removed — GetOrCreate should create a new one
		_, machine2, err := registry.GetOrCreate(ctx, jobID)
		require.NoError(t, err)
		assert.NotNil(t, machine2)
		// New machine should be a different instance
		assert.NotSame(t, machine, machine2)
	})
}
