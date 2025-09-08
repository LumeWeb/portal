package event

import "go.lumeweb.com/portal/core"

const (
	EVENT_INIT_START    = "init.start"
	EVENT_INIT_COMPLETE = "init.complete"
)

type InitStartEvent struct {
	Context core.Context
}

func NewInitStartEvent(ctx core.Context) *InitStartEvent {
	return &InitStartEvent{
		Context: ctx,
	}
}

type InitCompleteEvent struct {
	Context core.Context
}

func NewInitCompleteEvent(ctx core.Context) *InitCompleteEvent {
	return &InitCompleteEvent{
		Context: ctx,
	}
}

// OnInitStart registers a handler to run when the system init starts.
// This is a convenience wrapper around Listen for the EVENT_INIT_START event.
func OnInitStart(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[InitStartEvent](ctx, EVENT_INIT_START, func(e *core.CoreEvent[InitStartEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}

// OnInitComplete registers a handler to run when the system init completes.
// This is a convenience wrapper around Listen for the EVENT_INIT_COMPLETE event.
func OnInitComplete(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[InitCompleteEvent](ctx, EVENT_INIT_COMPLETE, func(e *core.CoreEvent[InitCompleteEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}
