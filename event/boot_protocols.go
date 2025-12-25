package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_PROTOCOLS           = "boot.protocols"
	EVENT_BOOT_PROTOCOLS_COMPLETED = "boot.protocols.completed"
)

type BootProtocolsEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootProtocolsEvent(ctx core.Context, eventCtx context.Context) *BootProtocolsEvent {
	return &BootProtocolsEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootProtocols registers a handler to run when the system boot protocols start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PROTOCOLS event.
func OnBootProtocols(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootProtocolsEvent](ctx, EVENT_BOOT_PROTOCOLS, func(e *core.CoreEvent[BootProtocolsEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootProtocolsCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootProtocolsCompletedEvent(ctx core.Context, eventCtx context.Context) *BootProtocolsCompletedEvent {
	return &BootProtocolsCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootProtocolsCompleted registers a handler to run when the system boot protocols completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PROTOCOLS_COMPLETED event.
func OnBootProtocolsCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootProtocolsCompletedEvent](ctx, EVENT_BOOT_PROTOCOLS_COMPLETED, func(e *core.CoreEvent[BootProtocolsCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
