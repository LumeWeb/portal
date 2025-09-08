package event

import (
	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_START     = "boot.start"
	EVENT_BOOT_COMPLETED = "boot.completed"
)

type BootStartEvent struct {
	Context core.Context
}

type BootCompletedEvent struct {
	Context core.Context
}

func NewBootCompletedEvent(ctx core.Context) *BootCompletedEvent {
	return &BootCompletedEvent{
		Context: ctx,
	}
}

func NewBootStartEvent(ctx core.Context) *BootStartEvent {
	return &BootStartEvent{
		Context: ctx,
	}
}


// OnBootStart registers a handler to run when the system boot starts.
// This is a convenience wrapper around Listen for the EVENT_BOOT_START event.
func OnBootStart(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[BootStartEvent](ctx, EVENT_BOOT_START, func(e *core.CoreEvent[BootStartEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}

// OnBootCompleted registers a handler to run when the system boot completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_COMPLETED event.
func OnBootCompleted(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[BootCompletedEvent](ctx, EVENT_BOOT_COMPLETED, func(e *core.CoreEvent[BootCompletedEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}
