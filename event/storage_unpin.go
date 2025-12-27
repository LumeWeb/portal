package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_UNPINNED = "storage.object.unpinned"

type StorageObjectUnpinnedEvent struct {
	Ctx context.Context
	Pin *models.Pin
	IP  string
}

func NewStorageObjectUnpinnedEvent(eventCtx context.Context, pin *models.Pin, ip string) *StorageObjectUnpinnedEvent {
	return &StorageObjectUnpinnedEvent{
		Ctx: eventCtx,
		Pin: pin,
		IP:  ip,
	}
}

// OnStorageObjectUnpinned registers a handler to run when a storage object is unpinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_UNPINNED event.
func OnStorageObjectUnpinned(ctx core.Context, handler func(context.Context, *models.Pin, string) error, priority ...int) {
	core.Listen[StorageObjectUnpinnedEvent](ctx, EVENT_STORAGE_OBJECT_UNPINNED, func(e *core.CoreEvent[StorageObjectUnpinnedEvent]) error {
		return handler(e.Data.Ctx, e.Data.Pin, e.Data.IP)
	}, priority...)
}
