package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const (
	EVENT_STORAGE_OBJECT_UNPINNED = "storage.object.unpinned"
)

func init() {
	core.RegisterEvent(EVENT_STORAGE_OBJECT_UNPINNED, &StorageObjectPinnedEvent{})
}

type StorageObjectUnpinnedEvent struct {
	core.Event
}

func (e *StorageObjectUnpinnedEvent) SetPin(metadata *models.Pin) {
	e.Set("pin", metadata)
}

func (e StorageObjectUnpinnedEvent) Pin() *models.Pin {
	return e.Get("pin").(*models.Pin)
}
func (e StorageObjectUnpinnedEvent) IP() string {
	return e.Get("ip").(string)
}

func FireStorageObjectUnpinnedEvent(ctx core.Context, pin *models.Pin) error {
	return Fire[*StorageObjectPinnedEvent](ctx, EVENT_STORAGE_OBJECT_UNPINNED, func(evt *StorageObjectPinnedEvent) error {
		evt.SetPin(pin)
		return nil
	})
}
