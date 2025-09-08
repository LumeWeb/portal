package event

import "go.lumeweb.com/portal/core"

const (
	EVENT_BOOT_STARTUP_FUNCS           = "boot.startup.funcs"
	EVENT_BOOT_STARTUP_FUNCS_COMPLETED = "boot.startup.funcs.completed"
)

type BootStartupFuncsEvent struct {
	Context core.Context
}

func NewBootStartupFuncsEvent(ctx core.Context) *BootStartupFuncsEvent {
	return &BootStartupFuncsEvent{
		Context: ctx,
	}
}

// OnBootStartupFuncs registers a handler to run when the system boot startup funcs start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_STARTUP_FUNCS event.
func OnBootStartupFuncs(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[BootStartupFuncsEvent](ctx, EVENT_BOOT_STARTUP_FUNCS, func(e *core.CoreEvent[BootStartupFuncsEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}

type BootStartupFuncsCompletedEvent struct {
	Context core.Context
}

func NewBootStartupFuncsCompletedEvent(ctx core.Context) *BootStartupFuncsCompletedEvent {
	return &BootStartupFuncsCompletedEvent{
		Context: ctx,
	}
}

// OnBootStartupFuncsCompleted registers a handler to run when the system boot startup funcs completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_STARTUP_FUNCS_COMPLETED event.
func OnBootStartupFuncsCompleted(ctx core.Context, handler func(core.Context) error, priority ...int) {
	core.Listen[BootStartupFuncsCompletedEvent](ctx, EVENT_BOOT_STARTUP_FUNCS_COMPLETED, func(e *core.CoreEvent[BootStartupFuncsCompletedEvent]) error {
		return handler(e.Data.Context)
	}, priority...)
}
