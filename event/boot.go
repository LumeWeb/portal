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
	evt, err := getEvent(ctx, EVENT_BOOT_COMPLETE)
	if err != nil {
		return err
	}

	configEvt, err := assertEventType[*BootCompleteEvent](evt, EVENT_BOOT_COMPLETE)
	if err != nil {
		return err
	}

	configEvt.SetContext(ctx)

	err = ctx.Event().FireEvent(configEvt)
	if err != nil {
		return err
	}

	return nil
}
