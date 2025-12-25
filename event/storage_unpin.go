package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_UNPINNED = "storage.object.unpinned"

type StorageObjectUnpinnedEvent struct {
	Pin *models.Pin
	IP  string
	Ctx context.Context
}

func NewStorageObjectUnpinnedEvent(pin *models.Pin, ip string, eventCtx context.Context) *StorageObjectUnpinnedEvent {
	return &StorageObjectUnpinnedEvent{
		Pin: pin,
		IP:  ip,
		Ctx: eventCtx,
	}
}

// OnStorageObjectUnpinned registers a handler to run when a storage object is unpinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_UNPINNED event.
func OnStorageObjectUnpinned(ctx core.Context, handler func(*models.Pin, string, context.Context) error, priority ...int) {
	core.Listen[StorageObjectUnpinnedEvent](ctx, EVENT_STORAGE_OBJECT_UNPINNED, func(e *core.CoreEvent[StorageObjectUnpinnedEvent]) error {
		return handler(e.Data.Pin, e.Data.IP, e.Data.Ctx)
	}, priority...)
}
