package event

import (
	"go.lumeweb.com/portal/core"
)

const (
	EVENT_BOOT_COMPLETE = "boot.complete"
)

func init() {
	core.RegisterEvent(EVENT_BOOT_COMPLETE, &BootCompleteEvent{})
}

type BootCompleteEvent struct {
	core.Event
}

func (e *BootCompleteEvent) SetContext(ctx core.Context) {
	e.Set("context", ctx)
}

func (e BootCompleteEvent) Context() core.Context {
	return e.Get("context").(core.Context)
}

func FireBootCompleteEvent(ctx core.Context) error {
	return Fire[*BootCompleteEvent](ctx, EVENT_BOOT_COMPLETE, func(evt *BootCompleteEvent) error {
		evt.SetContext(ctx)
		return nil
	})
}
