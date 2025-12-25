package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_HTTP           = "boot.http"
	EVENT_BOOT_HTTP_COMPLETED = "boot.http.completed"
)

type BootHTTPEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootHTTPEvent(ctx core.Context, eventCtx context.Context) *BootHTTPEvent {
	return &BootHTTPEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootHTTP registers a handler to run when the system boot http start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_HTTP event.
func OnBootHTTP(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootHTTPEvent](ctx, EVENT_BOOT_HTTP, func(e *core.CoreEvent[BootHTTPEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootHTTPCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootHTTPCompletedEvent(ctx core.Context, eventCtx context.Context) *BootHTTPCompletedEvent {
	return &BootHTTPCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootHTTPCompleted registers a handler to run when the system boot http completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_HTTP_COMPLETED event.
func OnBootHTTPCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootHTTPCompletedEvent](ctx, EVENT_BOOT_HTTP_COMPLETED, func(e *core.CoreEvent[BootHTTPCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
