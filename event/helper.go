package event

import (
	"fmt"
	"github.com/gookit/event"
	"go.lumeweb.com/portal/core"
)

// Helper function to get and check an event
func GetEvent(ctx core.Context, eventName string) (event.Event, error) {
	evt, ok := ctx.Event().GetEvent(eventName)
	if !ok {
		return nil, fmt.Errorf("event %s not found", eventName)
	}

	return evt, nil
}

func AssertEventType[T event.Event](evt event.Event, eventName string) (T, error) {
	typedEvt, ok := evt.(T)
	if !ok {
		return *new(T), fmt.Errorf("event %s is not of expected type", eventName)
	}
	return typedEvt, nil
}

func EventHandler[T event.Event](eventName string, handler func(T) error) event.ListenerFunc {
	return func(e event.Event) error {
		evt, err := AssertEventType[T](e, eventName)
		if err != nil {
			return err
		}
		return handler(evt)
	}
}

func SetupFire[T event.Event](
	ctx core.Context,
	eventName string,
) (T, error) {
	evt, err := GetEvent(ctx, eventName)
	if err != nil {
		return *new(T), err
	}

	configEvt, err := AssertEventType[T](evt, eventName)
	if err != nil {
		return *new(T), err
	}

	return configEvt, nil
}

func Fire[T event.Event](
	ctx core.Context,
	eventName string,
	cb func(evt T) error,
) error {
	evt, err := SetupFire[T](ctx, eventName)
	if err != nil {
		return err
	}

	if cb != nil {
		err = cb(evt)
		if err != nil {
			return err
		}
	}

	err = DoFire(ctx, evt)

	return err
}

func DoFire(
	ctx core.Context,
	event event.Event,
) error {
	return ctx.Event().FireEvent(event)
}

func Listen[T event.Event](
	ctx core.Context,
	eventName string,
	handler func(T) error,
) {
	ctx.Event().On(eventName, EventHandler(eventName, handler))
}
