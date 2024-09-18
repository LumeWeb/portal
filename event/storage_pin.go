package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const (
	EVENT_STORAGE_OBJECT_PINNED = "storage.object.pinned"
)

func init() {
	core.RegisterEvent(EVENT_STORAGE_OBJECT_PINNED, &StorageObjectPinnedEvent{})
}

type StorageObjectPinnedEvent struct {
	core.Event
}

func (e *StorageObjectPinnedEvent) SetPin(metadata *models.Pin) {
	e.Set("pin", metadata)
}

func (e StorageObjectPinnedEvent) Pin() *models.Pin {
	return e.Get("pin").(*models.Pin)
}

func (e *StorageObjectPinnedEvent) SetIP(ip string) {
	e.Set("ip", ip)
}

func (e StorageObjectPinnedEvent) IP() string {
	return e.Get("ip").(string)
}

func FireStorageObjectUploadedEvent(ctx core.Context, pin *models.Pin, ip string) error {
	return Fire[*StorageObjectPinnedEvent](ctx, EVENT_STORAGE_OBJECT_PINNED, func(evt *StorageObjectPinnedEvent) error {
		evt.SetPin(pin)
		evt.SetIP(ip)
		return nil
	})
}
