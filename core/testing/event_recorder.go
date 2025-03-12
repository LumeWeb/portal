package testing

import (
	"github.com/gookit/event"
	"go.lumeweb.com/portal/core"
	"sync"
)

// EventRecorder records events fired during tests
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

// Listen registers listeners for the specified event names
func (r *EventRecorder) Listen(ctx core.Context, eventNames ...string) {
	for _, name := range eventNames {
		// Use a closure to capture the event name
		eventName := name
		ctx.Event().On(eventName, event.ListenerFunc(func(e event.Event) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			
			if _, ok := r.events[eventName]; !ok {
				r.events[eventName] = make([]event.Event, 0)
			}
			r.events[eventName] = append(r.events[eventName], e)
			return nil
		}))
	}
}

// HasEvent checks if an event has been fired
func (r *EventRecorder) HasEvent(eventName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	events, ok := r.events[eventName]
	return ok && len(events) > 0
}

// GetEvents returns all recorded events for a given name
func (r *EventRecorder) GetEvents(eventName string) []event.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if events, ok := r.events[eventName]; ok {
		return events
	}
	return nil
}

// Reset clears all recorded events
func (r *EventRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.events = make(map[string][]event.Event)
}
