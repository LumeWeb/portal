package cron

import (
	"context"
	"fmt"
	"go.lumeweb.com/portal/db"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/looplab/fsm"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"gorm.io/gorm"
)

var _ core.CronJobStateMachineRegistry = (*DefaultStateMachineRegistry)(nil)

type DefaultStateMachineRegistry struct {
	machines map[uuid.UUID]*fsm.FSM
	ctx      core.Context
	mu       sync.RWMutex
	db       *gorm.DB
	logger   *core.Logger
}

func NewStateMachineRegistry(ctx core.Context) *DefaultStateMachineRegistry {
	return &DefaultStateMachineRegistry{
		machines: make(map[uuid.UUID]*fsm.FSM),
		ctx:      ctx,
		db:       ctx.DB(),
		logger:   ctx.Logger(),
	}
}

func (r *DefaultStateMachineRegistry) GetOrCreate(ctx context.Context, jobID uuid.UUID) (*models.CronJob, *fsm.FSM, error) {
	ctx, span := core.TraceMethod(ctx, "DefaultStateMachineRegistry.GetOrCreate")
	defer span.End()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get current job state from DB first
	var job models.CronJob
	if err := r.db.Where("uuid = ?", types.FromUUID(jobID)).First(&job).Error; err != nil {
		r.logger.Error("Failed to get job from DB",
			zap.String("jobID", jobID.String()),
			zap.Error(err))
		return nil, nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Check for existing machine after getting fresh job state
	if machine, exists := r.machines[jobID]; exists {
		return &job, machine, nil
	}

	events := createFSMEvents()

	machine := fsm.NewFSM(
		string(job.State),
		events,
		fsm.Callbacks{
			"after_event": r.afterEventCallback,
		},
	)

	r.machines[jobID] = machine
	return &job, machine, nil
}

func (r *DefaultStateMachineRegistry) persistState(ctx context.Context, jobID uuid.UUID, currentVersion int64, newState models.CronJobState, params *core.CronStateParams) error {
	ctx, span := core.TraceMethod(ctx, "DefaultStateMachineRegistry.persistState")
	defer span.End()

	return db.RetryableTransaction(r.ctx, r.db, func(tx *gorm.DB) *gorm.DB {
		updates := map[string]interface{}{
			"state":   newState,
			"version": currentVersion + 1,
		}

		now := time.Now()
		switch {
		case newState == models.CronJobStateRunning:
			updates["last_run"] = now
			updates["last_heartbeat"] = now
		case params.LastRun():
			updates["last_run"] = now
			updates["last_heartbeat"] = now
		case params.Heartbeat():
			updates["last_heartbeat"] = now
		}

		if params.Failures() >= 0 {
			updates["failures"] = params.Failures()
		}

		// Perform update with optimistic locking
		result := tx.WithContext(ctx).
			Model(&models.CronJob{}).
			Where("uuid = ? AND version = ?", types.FromUUID(jobID), currentVersion).
			Updates(updates)

		if result.Error != nil {
			_ = tx.AddError(fmt.Errorf("failed to update job state: %w", result.Error))
		}

		if result.RowsAffected == 0 {
			if err := r.handleVersionConflict(ctx, jobID); err != nil {
				_ = tx.AddError(fmt.Errorf("version conflict: %w", err))
			} else {
				_ = tx.AddError(ErrCronJobVersionConflict)
			}
		}

		return tx
	})
}

func (r *DefaultStateMachineRegistry) handleVersionConflict(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := core.TraceMethod(ctx, "DefaultStateMachineRegistry.handleVersionConflict")
	defer span.End()

	var exists bool
	err := r.db.WithContext(ctx).
		Model(&models.CronJob{}).
		Select("1").
		Where("uuid = ?", types.FromUUID(jobID)).
		Limit(1).
		Find(&exists).Error
	if err != nil {
		return fmt.Errorf("failed to check job existence: %w", err)
	}
	if !exists {
		return ErrCronJobNotFound
	}
	return nil
}

func (r *DefaultStateMachineRegistry) afterEventCallback(ctx context.Context, e *fsm.Event) {
	ctx, span := core.TraceMethod(ctx, "DefaultStateMachineRegistry.afterEventCallback")
	defer span.End()

	data, ok := ctx.Value(stateMachineDataKey).(*stateMachineData)
	if !ok {
		e.Cancel(fmt.Errorf("missing state machine data"))
		return
	}

	// Ensure params is initialized if nil
	if data.params == nil {
		data.params = &core.CronStateParams{}
	}

	// For transitions to running state, always set heartbeat and last run
	if models.CronJobState(e.Dst) == models.CronJobStateRunning {
		core.WithCronHeartbeat()(data.params)
		core.WithCronLastRun()(data.params)
	}

	err := r.persistState(ctx, data.jobID, data.currentVersion, models.CronJobState(e.Dst), data.params)
	if err != nil {
		e.Cancel(fmt.Errorf("failed to persist state: %w", err))
	}
}

func (r *DefaultStateMachineRegistry) Remove(ctx context.Context, jobID uuid.UUID) {
	ctx, span := core.TraceMethod(ctx, "DefaultStateMachineRegistry.Remove")
	defer span.End()

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.machines, jobID)
}
