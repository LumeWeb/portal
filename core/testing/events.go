package testing

import (
	"github.com/gookit/event"
	"go.lumeweb.com/portal/core"
	"sync"
)

// EventRecorder records events that were fired
type EventRecorder struct {
	events map[string][]event.Event
	mu     sync.RWMutex
}

// NewEventRecorder creates a new event recorder
func NewEventRecorder() *EventRecorder {
	return &EventRecorder{
		events: make(map[string][]event.Event),
	}
}

// Listen starts listening for events
func (r *EventRecorder) Listen(ctx core.Context, eventNames ...string) {
	for _, name := range eventNames {
		ctx.Event().On(name, event.ListenerFunc(func(e event.Event) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			
			r.events[name] = append(r.events[name], e)
			return nil
		}), event.Normal)
	}
}

// GetEvents returns all recorded events for a given name
func (r *EventRecorder) GetEvents(name string) []event.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return r.events[name]
}

// Reset clears all recorded events
func (r *EventRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.events = make(map[string][]event.Event)
}

// HasEvent checks if an event was fired
func (r *EventRecorder) HasEvent(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	events, ok := r.events[name]
	return ok && len(events) > 0
}

// Count returns the number of events recorded for a given name
func (r *EventRecorder) Count(name string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return len(r.events[name])
}
