package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_PINNED = "storage.object.pinned"

type StorageObjectPinnedEvent struct {
	Pin *models.Pin
	IP  string
}

func NewStorageObjectPinnedEvent(pin *models.Pin, ip string) *StorageObjectPinnedEvent {
	return &StorageObjectPinnedEvent{
		Pin: pin,
		IP:  ip,
	}
}

// OnStorageObjectPinned registers a handler to run when a storage object is pinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_PINNED event.
func OnStorageObjectPinned(ctx core.Context, handler func(*models.Pin, string) error, priority ...int) {
	core.Listen[StorageObjectPinnedEvent](ctx, EVENT_STORAGE_OBJECT_PINNED, func(e *core.CoreEvent[StorageObjectPinnedEvent]) error {
		return handler(e.Data.Pin, e.Data.IP)
	}, priority...)
}
