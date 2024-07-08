package event

import (
	"fmt"
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
	evt, ok := ctx.Event().GetEvent(EVENT_BOOT_COMPLETE)
	if !ok {
		return fmt.Errorf("event %s not found", EVENT_BOOT_COMPLETE)
	}

	bootEvt, ok := evt.(*BootCompleteEvent)
	if !ok {
		return fmt.Errorf("event %s is not of type BootCompleteEvent", EVENT_BOOT_COMPLETE)
	}

	bootEvt.SetContext(ctx)

	err := ctx.Event().FireEvent(bootEvt)
	if err != nil {
		return err
	}

	return nil
}
