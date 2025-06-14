package event

import "go.lumeweb.com/portal/db/models"

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
