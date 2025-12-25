package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_PROTOCOL_WORKFLOWS           = "boot.protocol.workflows"
	EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED = "boot.protocol.workflows.completed"
)

type BootProtocolWorkflowsEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootProtocolWorkflowsEvent(ctx core.Context, eventCtx context.Context) *BootProtocolWorkflowsEvent {
	return &BootProtocolWorkflowsEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootProtocolWorkflows registers a handler to run when the system boot protocol workflows start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PROTOCOL_WORKFLOWS event.
func OnBootProtocolWorkflows(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootProtocolWorkflowsEvent](ctx, EVENT_BOOT_PROTOCOL_WORKFLOWS, func(e *core.CoreEvent[BootProtocolWorkflowsEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootProtocolWorkflowsCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootProtocolWorkflowsCompletedEvent(ctx core.Context, eventCtx context.Context) *BootProtocolWorkflowsCompletedEvent {
	return &BootProtocolWorkflowsCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootProtocolWorkflowsCompleted registers a handler to run when the system boot protocol workflows completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED event.
func OnBootProtocolWorkflowsCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootProtocolWorkflowsCompletedEvent](ctx, EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED, func(e *core.CoreEvent[BootProtocolWorkflowsCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
