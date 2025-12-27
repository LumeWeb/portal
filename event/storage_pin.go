package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_PINNED = "storage.object.pinned"

type StorageObjectPinnedEvent struct {
	Ctx context.Context
	Pin *models.Pin
	IP  string
}

func NewStorageObjectPinnedEvent(eventCtx context.Context, pin *models.Pin, ip string) *StorageObjectPinnedEvent {
	return &StorageObjectPinnedEvent{
		Ctx: eventCtx,
		Pin: pin,
		IP:  ip,
	}
}

// OnStorageObjectPinned registers a handler to run when a storage object is pinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_PINNED event.
func OnStorageObjectPinned(ctx core.Context, handler func(context.Context, *models.Pin, string) error, priority ...int) {
	core.Listen[StorageObjectPinnedEvent](ctx, EVENT_STORAGE_OBJECT_PINNED, func(e *core.CoreEvent[StorageObjectPinnedEvent]) error {
		return handler(e.Data.Ctx, e.Data.Pin, e.Data.IP)
	}, priority...)
}
