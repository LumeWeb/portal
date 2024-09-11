package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const (
	EVENT_STORAGE_OBJECT_UPLOADED = "storage.object.uploaded"
)

func init() {
	core.RegisterEvent(EVENT_STORAGE_OBJECT_UPLOADED, &StorageObjectUploadedEvent{})
}

type StorageObjectUploadedEvent struct {
	core.Event
}

func (e *StorageObjectUploadedEvent) SetPin(metadata *models.Pin) {
	e.Set("pin", metadata)
}

func (e StorageObjectUploadedEvent) Pin() *models.Pin {
	return e.Get("pin").(*models.Pin)
}

func FireStorageObjectUploadedEvent(ctx core.Context, pin *models.Pin) error {
	return Fire[*StorageObjectUploadedEvent](ctx, EVENT_STORAGE_OBJECT_UPLOADED, func(evt *StorageObjectUploadedEvent) error {
		evt.SetPin(pin)
		return nil
	})
}
