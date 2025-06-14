package event

import (
	"go.lumeweb.com/portal/core"
)

const EVENT_BOOT_COMPLETE = "boot.complete"

type BootCompleteEvent struct {
	Context core.Context
}

func NewBootCompleteEvent(ctx core.Context) *BootCompleteEvent {
	return &BootCompleteEvent{
		Context: ctx,
	}
}
