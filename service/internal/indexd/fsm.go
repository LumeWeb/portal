package indexd

import (
	"context"
	"fmt"

	"github.com/looplab/fsm"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

// SiaObjectEvent is the FSM event type for RenterObject state transitions.
type SiaObjectEvent string

const (
	EventPack   SiaObjectEvent = "pack"   // staged -> packing
	EventUpload SiaObjectEvent = "upload" // packing -> uploaded
	EventDelete SiaObjectEvent = "delete" // staged|uploaded -> deleting
	EventRevert SiaObjectEvent = "revert" // packing -> staged
)

// siaObjectStateToEvent maps each target state to the FSM event that triggers it.
var siaObjectStateToEvent = map[models.RenterObjectStatus]SiaObjectEvent{
	models.RenterObjectStatusPacking:  EventPack,
	models.RenterObjectStatusUploaded: EventUpload,
	models.RenterObjectStatusDeleting: EventDelete,
	models.RenterObjectStatusStaged:   EventRevert,
}

// siaObjectStateTransitions defines valid source states for each destination.
var siaObjectStateTransitions = map[models.RenterObjectStatus][]string{
	models.RenterObjectStatusPacking: {
		string(models.RenterObjectStatusStaged),
	},
	models.RenterObjectStatusUploaded: {
		string(models.RenterObjectStatusPacking),
	},
	models.RenterObjectStatusDeleting: {
		string(models.RenterObjectStatusStaged),
		string(models.RenterObjectStatusUploaded),
	},
	models.RenterObjectStatusStaged: {
		string(models.RenterObjectStatusPacking),
	},
}

// createSiaObjectFSMEvents builds the FSM event descriptors from the
// declarative state maps, following the portal's cron FSM pattern.
func createSiaObjectFSMEvents() fsm.Events {
	events := make(fsm.Events, 0, len(siaObjectStateToEvent))
	for state, event := range siaObjectStateToEvent {
		src, ok := siaObjectStateTransitions[state]
		if !ok {
			continue
		}
		events = append(events, fsm.EventDesc{
			Name: string(event),
			Src:  src,
			Dst:  string(state),
		})
	}
	return events
}

// newSiaObjectFSM creates a looplab FSM seeded with the given current state.
func newSiaObjectFSM(current models.RenterObjectStatus) *fsm.FSM {
	return fsm.NewFSM(
		string(current),
		createSiaObjectFSMEvents(),
		fsm.Callbacks{},
	)
}

// canTransition checks whether a transition from current to newState is valid
// according to the FSM definition. The FSM validates that the source state
// is a legal origin for the destination state.
func CanTransition(current, newState models.RenterObjectStatus) bool {
	fsmInstance := newSiaObjectFSM(current)
	event, ok := siaObjectStateToEvent[newState]
	if !ok {
		return false
	}
	return fsmInstance.Can(string(event))
}

// TransitionState performs a CAS state transition on the RenterObject with the
// given ID. It first validates the transition is legal via the FSM, then
// issues a conditional UPDATE that only succeeds if the current DB status
// matches the expected current state.
//
// Returns:
// - rowsAffected == 0, nil: object was modified by another goroutine (race)
// - rowsAffected == 1, nil: transition succeeded
// - 0, error: database error or invalid transition
func TransitionState(component core.Component, ctx context.Context, id uint, current, newState models.RenterObjectStatus) (int64, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.TransitionState")
	defer span.End()
	span.SetAttributes(
		attribute.Int64("indexd.objectId", int64(id)),
		attribute.String("indexd.currentState", string(current)),
		attribute.String("indexd.newState", string(newState)),
	)
	if !CanTransition(current, newState) {
		return 0, fmt.Errorf("invalid transition: %s -> %s", current, newState)
	}

	var rowsAffected int64
	if err := db.RetryableComponentTransaction(component, ctx, func(tx *gorm.DB) *gorm.DB {
		result := tx.Model(&models.RenterObject{}).
			Where("id = ? AND status = ?", id, current).
			Update("status", newState)
		rowsAffected = result.RowsAffected
		return result
	}); err != nil {
		return 0, fmt.Errorf("failed to CAS %s->%s: %w", current, newState, err)
	}

	return rowsAffected, nil
}

// TransitionStateWithUpdates performs a CAS state transition and applies
// additional column updates in the same transaction. Used when transitioning
// to "uploaded" and needing to set sia_object_id, sealed_data, and clear
// staging_key atomically with the status change.
func TransitionStateWithUpdates(component core.Component, ctx context.Context, id uint, current, newState models.RenterObjectStatus, updates map[string]interface{}) (int64, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.TransitionStateWithUpdates")
	defer span.End()
	span.SetAttributes(
		attribute.Int64("indexd.objectId", int64(id)),
		attribute.String("indexd.currentState", string(current)),
		attribute.String("indexd.newState", string(newState)),
		attribute.Int("indexd.updateFieldCount", len(updates)),
	)
	if !CanTransition(current, newState) {
		return 0, fmt.Errorf("invalid transition: %s -> %s", current, newState)
	}

	// Merge the status into the updates map.
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["status"] = newState

	var rowsAffected int64
	if err := db.RetryableComponentTransaction(component, ctx, func(tx *gorm.DB) *gorm.DB {
		result := tx.Model(&models.RenterObject{}).
			Where("id = ? AND status = ?", id, current).
			Updates(updates)
		rowsAffected = result.RowsAffected
		return result
	}); err != nil {
		return 0, fmt.Errorf("failed to CAS %s->%s with updates: %w", current, newState, err)
	}

	return rowsAffected, nil
}
