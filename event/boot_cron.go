package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_CRON           = "boot.cron"
	EVENT_BOOT_CRON_COMPLETED = "boot.cron.completed"
)

type BootCronEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootCronEvent(ctx core.Context, eventCtx context.Context) *BootCronEvent {
	return &BootCronEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootCron registers a handler to run when the system boot cron start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_CRON event.
func OnBootCron(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootCronEvent](ctx, EVENT_BOOT_CRON, func(e *core.CoreEvent[BootCronEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootCronCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootCronCompletedEvent(ctx core.Context, eventCtx context.Context) *BootCronCompletedEvent {
	return &BootCronCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootCronCompleted registers a handler to run when the system boot cron completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_CRON_COMPLETED event.
func OnBootCronCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootCronCompletedEvent](ctx, EVENT_BOOT_CRON_COMPLETED, func(e *core.CoreEvent[BootCronCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
