package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_MAILER           = "boot.mailer"
	EVENT_BOOT_MAILER_COMPLETED = "boot.mailer.completed"
)

type BootMailerEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootMailerEvent(ctx core.Context, eventCtx context.Context) *BootMailerEvent {
	return &BootMailerEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootMailer registers a handler to run when the system boot mailer start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_MAILER event.
func OnBootMailer(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootMailerEvent](ctx, EVENT_BOOT_MAILER, func(e *core.CoreEvent[BootMailerEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootMailerCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootMailerCompletedEvent(ctx core.Context, eventCtx context.Context) *BootMailerCompletedEvent {
	return &BootMailerCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootMailerCompleted registers a handler to run when the system boot mailer completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_MAILER_COMPLETED event.
func OnBootMailerCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootMailerCompletedEvent](ctx, EVENT_BOOT_MAILER_COMPLETED, func(e *core.CoreEvent[BootMailerCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
