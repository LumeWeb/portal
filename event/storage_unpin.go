package event

import (
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
