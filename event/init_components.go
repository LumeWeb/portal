package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_INIT_COMPONENTS_REGISTERED  = "init.components.registered"
	EVENT_INIT_COMPONENTS_CONFIGURED = "init.components.configured"
)

type InitComponentsRegisteredEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewInitComponentsRegisteredEvent(ctx core.Context, eventCtx context.Context) *InitComponentsRegisteredEvent {
	return &InitComponentsRegisteredEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

type InitComponentsConfiguredEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewInitComponentsConfiguredEvent(ctx core.Context, eventCtx context.Context) *InitComponentsConfiguredEvent {
	return &InitComponentsConfiguredEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnInitComponentsRegistered registers a handler to run when system components are registered.
// This is a convenience wrapper around Listen for the EVENT_INIT_COMPONENTS_REGISTERED event.
func OnInitComponentsRegistered(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[InitComponentsRegisteredEvent](ctx, EVENT_INIT_COMPONENTS_REGISTERED, func(e *core.CoreEvent[InitComponentsRegisteredEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

// OnInitComponentsConfigured registers a handler to run when system components are configured.
// This is a convenience wrapper around Listen for the EVENT_INIT_COMPONENTS_CONFIGURED event.
func OnInitComponentsConfigured(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[InitComponentsConfiguredEvent](ctx, EVENT_INIT_COMPONENTS_CONFIGURED, func(e *core.CoreEvent[InitComponentsConfiguredEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
