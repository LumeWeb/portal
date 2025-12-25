package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_INIT_DATABASE_READY = "init.database.ready"

type InitDatabaseReadyEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewInitDatabaseReadyEvent(ctx core.Context, eventCtx context.Context) *InitDatabaseReadyEvent {
	return &InitDatabaseReadyEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnInitDatabaseReady registers a handler to run when the database is ready.
// This is a convenience wrapper around Listen for the EVENT_INIT_DATABASE_READY event.
func OnInitDatabaseReady(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[InitDatabaseReadyEvent](ctx, EVENT_INIT_DATABASE_READY, func(e *core.CoreEvent[InitDatabaseReadyEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
