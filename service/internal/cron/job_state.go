package cron

// Package cron implements a finite state machine (FSM) for managing cron job states.
// The FSM defines valid state transitions and events that trigger those transitions.
//
// State Machine Overview:
// - States: Queued, Running, Completed, Failed
// - Events: run, complete, fail, reset
// - Transitions:
//   Queued -> Running (via "run" event)
//   Running -> Completed (via "complete" event)
//   Running -> Failed (via "fail" event)
//   Queued -> Failed (via "fail" event, rollback from failed requeue)
//   Completed -> Queued (via "reset" event)
//   Failed -> Queued (via "reset" event)
//
// Extending the State Machine:
// 1. Add new states to models.CronJobState
// 2. Add state-to-event mapping in stateToEvent
// 3. Define valid transitions in stateTransitions
// 4. Add corresponding event handlers if needed
//
// The FSM ensures only valid transitions can occur and persists state changes to the database.

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/looplab/fsm"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var (
	// ErrCronJobNotFound is returned when a cron job cannot be found in the database.
	ErrCronJobNotFound = errors.New("cron job not found")

	// ErrCronJobVersionConflict is returned when a state transition fails due to
	// optimistic locking — another process modified the job between read and write.
	ErrCronJobVersionConflict = errors.New("cron job version conflict - concurrent modification detected")

	// stateToEvent maps each target state to the event name that triggers it.
	// This allows looking up the FSM event name needed to reach a desired state.
	// Example: To transition to Running state, send the "run" event.
	stateToEvent = map[models.CronJobState]string{
		models.CronJobStateRunning:   "run",
		models.CronJobStateCompleted: "complete",
		models.CronJobStateFailed:    "fail",
		models.CronJobStateQueued:    "reset",
	}

	// stateTransitions defines valid state transitions.
	// Each key is a destination state, and its value is a list of valid source states.
	// This structure is used to generate FSM events where:
	// - The event name comes from stateToEvent
	// - The source states come from this map
	// - The destination state is the map key
	stateTransitions = map[models.CronJobState][]string{
		models.CronJobStateRunning: {
			string(models.CronJobStateQueued), // Only Queued can go to Running
		},
		models.CronJobStateCompleted: {
			string(models.CronJobStateRunning), // Only Running can go to Completed
		},
		models.CronJobStateFailed: {
			string(models.CronJobStateRunning), // Running can fail
			string(models.CronJobStateQueued),  // Queued can be rolled back to Failed
		},
		models.CronJobStateQueued: {
			string(models.CronJobStateCompleted), // Completed can reset to Queued
			string(models.CronJobStateFailed),    // Failed can reset to Queued
		},
	}
)

// createFSMEvents generates FSM event descriptors from the stateToEvent and stateTransitions mappings.
// This converts our declarative state machine definition into the format expected by the FSM library.
func createFSMEvents() fsm.Events {
	events := make(fsm.Events, 0, len(stateToEvent))
	for state, event := range stateToEvent {
		src, ok := stateTransitions[state]
		if !ok {
			continue
		}
		events = append(events, fsm.EventDesc{
			Name: event,
			Src:  src,
			Dst:  string(state),
		})
	}
	return events
}

// DefaultCronJobStateMachine implements core.CronJobStateMachine interface.
// It manages state transitions for cron jobs using a finite state machine.
// The machine ensures only valid transitions occur and persists state changes.
type DefaultCronJobStateMachine struct {
	db       *gorm.DB
	ctx      core.Context
	registry core.CronJobStateMachineRegistry
	fsm      *fsm.FSM
}

var _ core.CronJobStateMachine = (*DefaultCronJobStateMachine)(nil)

func NewCronJobStateMachine(ctx core.Context, registry core.CronJobStateMachineRegistry) *DefaultCronJobStateMachine {
	return &DefaultCronJobStateMachine{
		db:       ctx.DB(),
		ctx:      ctx,
		registry: registry,
	}
}

// stateMachineDataKeyType is used as a unique context key for passing state machine data
// between transition methods and callbacks.
type stateMachineDataKeyType struct{}

var stateMachineDataKey = stateMachineDataKeyType{}

// stateMachineData contains the job context needed during state transitions.
// This data is passed via context to callbacks and includes:
// - jobID: The UUID of the job being transitioned
// - currentVersion: The job's version for optimistic locking
// - params: Additional state parameters (last run, failures, etc)
type stateMachineData struct {
	jobID          uuid.UUID
	currentVersion int64
	params         *core.CronStateParams
}

// Transition validates and performs state transitions
func (sm *DefaultCronJobStateMachine) Transition(ctx context.Context, jobID uuid.UUID, newState models.CronJobState, opts ...core.CronStateOption) error {
	ctx, span := core.TraceMethod(ctx, "DefaultCronJobStateMachine.Transition")
	defer span.End()

	// Get job and FSM from registry
	job, fsmInstance, err := sm.registry.GetOrCreate(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get state machine: %w", err)
	}
	sm.fsm = fsmInstance

	// Apply options
	params := &core.CronStateParams{}
	for _, opt := range opts {
		opt(params)
	}

	// Get event name from state mapping
	event, ok := stateToEvent[newState]
	if !ok {
		return fmt.Errorf("invalid target state: %s", newState)
	}

	// Update FSM's internal state to match DB
	fsmInstance.SetState(string(job.State))

	// Check if the event is allowed from the current state
	if !fsmInstance.Can(event) {
		return fmt.Errorf("invalid transition from %s to %s", job.State, newState)
	}

	// Store data in context for callback
	data := &stateMachineData{
		jobID:          jobID,
		currentVersion: job.Version,
		params:         params,
	}
	ctx = context.WithValue(ctx, stateMachineDataKey, data)

	// Perform the state transition
	if err = fsmInstance.Event(ctx, event); err != nil {
		return fmt.Errorf("failed to transition state: %w", err)
	}

	return nil
}

// State returns the current state of the FSM.
func (sm *DefaultCronJobStateMachine) State() string {
	if sm.fsm == nil {
		return string(models.CronJobStateQueued) // Default state if FSM not initialized
	}
	return sm.fsm.Current()
}

// IsValidTransition checks if a state transition is valid
func (sm *DefaultCronJobStateMachine) RemoveStateMachine(ctx context.Context, jobID uuid.UUID) {
	ctx, span := core.TraceMethod(ctx, "DefaultCronJobStateMachine.RemoveStateMachine")
	defer span.End()

	sm.registry.Remove(ctx, jobID)
}

func (sm *DefaultCronJobStateMachine) IsValidTransition(ctx context.Context, current, new models.CronJobState) bool {
	ctx, span := core.TraceMethod(ctx, "DefaultCronJobStateMachine.IsValidTransition")
	defer span.End()

	// Get allowed source states for the destination state
	allowedSrcs, ok := stateTransitions[new]
	if !ok {
		return false
	}

	// Check if current state is in allowed sources
	for _, src := range allowedSrcs {
		if src == string(current) {
			return true
		}
	}
	return false
}
