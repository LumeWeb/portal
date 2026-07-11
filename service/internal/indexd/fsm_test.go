package indexd

import (
	"testing"

	"go.lumeweb.com/portal/db/models"
)

func TestCanTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current models.RenterObjectStatus
		target  models.RenterObjectStatus
	}{
		{"staged to packing", models.RenterObjectStatusStaged, models.RenterObjectStatusPacking},
		{"packing to uploaded", models.RenterObjectStatusPacking, models.RenterObjectStatusUploaded},
		{"packing to staged (revert)", models.RenterObjectStatusPacking, models.RenterObjectStatusStaged},
		{"staged to deleting", models.RenterObjectStatusStaged, models.RenterObjectStatusDeleting},
		{"uploaded to deleting", models.RenterObjectStatusUploaded, models.RenterObjectStatusDeleting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !CanTransition(tt.current, tt.target) {
				t.Errorf("CanTransition(%s, %s) = false, want true", tt.current, tt.target)
			}
		})
	}
}

func TestCanTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current models.RenterObjectStatus
		target  models.RenterObjectStatus
	}{
		{"staged to uploaded (skip packing)", models.RenterObjectStatusStaged, models.RenterObjectStatusUploaded},
		{"uploaded to packing", models.RenterObjectStatusUploaded, models.RenterObjectStatusPacking},
		{"uploaded to staged", models.RenterObjectStatusUploaded, models.RenterObjectStatusStaged},
		{"packing to deleting", models.RenterObjectStatusPacking, models.RenterObjectStatusDeleting},
		{"deleting to staged", models.RenterObjectStatusDeleting, models.RenterObjectStatusStaged},
		{"deleting to packing", models.RenterObjectStatusDeleting, models.RenterObjectStatusPacking},
		{"deleting to uploaded", models.RenterObjectStatusDeleting, models.RenterObjectStatusUploaded},
		{"staged to staged (no-op)", models.RenterObjectStatusStaged, models.RenterObjectStatusStaged},
		{"packing to packing (no-op)", models.RenterObjectStatusPacking, models.RenterObjectStatusPacking},
		{"uploaded to uploaded (no-op)", models.RenterObjectStatusUploaded, models.RenterObjectStatusUploaded},
		{"deleting to deleting (no-op)", models.RenterObjectStatusDeleting, models.RenterObjectStatusDeleting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanTransition(tt.current, tt.target) {
				t.Errorf("CanTransition(%s, %s) = true, want false", tt.current, tt.target)
			}
		})
	}
}

func TestSiaObjectEvent_Constants(t *testing.T) {
	// Verify events are distinct.
	events := []SiaObjectEvent{EventPack, EventUpload, EventDelete, EventRevert}
	seen := make(map[SiaObjectEvent]bool)
	for _, e := range events {
		if e == "" {
			t.Fatalf("event constant is empty string")
		}
		if seen[e] {
			t.Fatalf("duplicate event value: %s", e)
		}
		seen[e] = true
	}
}

func TestSiaObjectStateToEvent_AllStatesCovered(t *testing.T) {
	// Every status except the current state itself must have a route in the map.
	// The FSM should be able to reach every state from at least one source.
	targetStates := []models.RenterObjectStatus{
		models.RenterObjectStatusPacking,
		models.RenterObjectStatusUploaded,
		models.RenterObjectStatusDeleting,
		models.RenterObjectStatusStaged,
	}
	for _, target := range targetStates {
		_, ok := siaObjectStateToEvent[target]
		if !ok {
			t.Errorf("state %s has no event in siaObjectStateToEvent map", target)
		}
		sources, ok := siaObjectStateTransitions[target]
		if !ok || len(sources) == 0 {
			t.Errorf("state %s has no source states in siaObjectStateTransitions map", target)
		}
	}
}
