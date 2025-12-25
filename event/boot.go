package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_START     = "boot.start"
	EVENT_BOOT_COMPLETED = "boot.completed"
)

type BootStartEvent struct {
	Context core.Context
	Ctx     context.Context
}

type BootCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootCompletedEvent(ctx core.Context, eventCtx context.Context) *BootCompletedEvent {
	return &BootCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

func NewBootStartEvent(ctx core.Context, eventCtx context.Context) *BootStartEvent {
	return &BootStartEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}


// OnBootStart registers a handler to run when the system boot starts.
// This is a convenience wrapper around Listen for the EVENT_BOOT_START event.
func OnBootStart(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootStartEvent](ctx, EVENT_BOOT_START, func(e *core.CoreEvent[BootStartEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

// OnBootCompleted registers a handler to run when the system boot completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_COMPLETED event.
func OnBootCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootCompletedEvent](ctx, EVENT_BOOT_COMPLETED, func(e *core.CoreEvent[BootCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
