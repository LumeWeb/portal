package event

import (
	"fmt"
	"go.lumeweb.com/portal/core"
)

// Helper function to get and check an event
func getEvent(ctx core.Context, eventName string) (core.Eventer, error) {
	evt, ok := ctx.Event().GetEvent(eventName)
	if !ok {
		return nil, fmt.Errorf("event %s not found", eventName)
	}

	return evt.(core.Eventer), nil
}

// Helper function to assert event type
func assertEventType[T core.Eventer](evt core.Eventer, eventName string) (T, error) {
	typedEvt, ok := evt.(T)
	if !ok {
		return *new(T), fmt.Errorf("event %s is not of expected type", eventName)
	}
	return typedEvt, nil
}
