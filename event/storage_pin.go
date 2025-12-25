package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_PINNED = "storage.object.pinned"

type StorageObjectPinnedEvent struct {
	Pin *models.Pin
	IP  string
	Ctx context.Context
}

func NewStorageObjectPinnedEvent(pin *models.Pin, ip string, eventCtx context.Context) *StorageObjectPinnedEvent {
	return &StorageObjectPinnedEvent{
		Pin: pin,
		IP:  ip,
		Ctx: eventCtx,
	}
}

// OnStorageObjectPinned registers a handler to run when a storage object is pinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_PINNED event.
func OnStorageObjectPinned(ctx core.Context, handler func(*models.Pin, string, context.Context) error, priority ...int) {
	core.Listen[StorageObjectPinnedEvent](ctx, EVENT_STORAGE_OBJECT_PINNED, func(e *core.CoreEvent[StorageObjectPinnedEvent]) error {
		return handler(e.Data.Pin, e.Data.IP, e.Data.Ctx)
	}, priority...)
}
