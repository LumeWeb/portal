package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_STORAGE_OBJECT_UNPINNED = "storage.object.unpinned"

type StorageObjectUnpinnedEvent struct {
	Pin *models.Pin
	IP  string
}

func NewStorageObjectUnpinnedEvent(pin *models.Pin, ip string) *StorageObjectUnpinnedEvent {
	return &StorageObjectUnpinnedEvent{
		Pin: pin,
		IP:  ip,
	}
}

// OnStorageObjectUnpinned registers a handler to run when a storage object is unpinned.
// This is a convenience wrapper around Listen for the EVENT_STORAGE_OBJECT_UNPINNED event.
func OnStorageObjectUnpinned(ctx core.Context, handler func(*models.Pin, string) error, priority ...int) {
	core.Listen[StorageObjectUnpinnedEvent](ctx, EVENT_STORAGE_OBJECT_UNPINNED, func(e *core.CoreEvent[StorageObjectUnpinnedEvent]) error {
		return handler(e.Data.Pin, e.Data.IP)
	}, priority...)
}
