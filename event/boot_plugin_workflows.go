package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_PLUGIN_WORKFLOWS           = "boot.plugin.workflows"
	EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED = "boot.plugin.workflows.completed"
)

type BootPluginWorkflowsEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootPluginWorkflowsEvent(ctx core.Context, eventCtx context.Context) *BootPluginWorkflowsEvent {
	return &BootPluginWorkflowsEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootPluginWorkflows registers a handler to run when the system boot plugin workflows start.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PLUGIN_WORKFLOWS event.
func OnBootPluginWorkflows(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootPluginWorkflowsEvent](ctx, EVENT_BOOT_PLUGIN_WORKFLOWS, func(e *core.CoreEvent[BootPluginWorkflowsEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}

type BootPluginWorkflowsCompletedEvent struct {
	Context core.Context
	Ctx     context.Context
}

func NewBootPluginWorkflowsCompletedEvent(ctx core.Context, eventCtx context.Context) *BootPluginWorkflowsCompletedEvent {
	return &BootPluginWorkflowsCompletedEvent{
		Context: ctx,
		Ctx:     eventCtx,
	}
}

// OnBootPluginWorkflowsCompleted registers a handler to run when the system boot plugin workflows completes.
// This is a convenience wrapper around Listen for the EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED event.
func OnBootPluginWorkflowsCompleted(ctx core.Context, handler func(core.Context, context.Context) error, priority ...int) {
	core.Listen[BootPluginWorkflowsCompletedEvent](ctx, EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED, func(e *core.CoreEvent[BootPluginWorkflowsCompletedEvent]) error {
		return handler(e.Data.Context, e.Data.Ctx)
	}, priority...)
}
