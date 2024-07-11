package event

import (
	"go.lumeweb.com/portal/core"
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

func (e *StorageObjectUploadedEvent) SetObjectMetadata(metadata *core.UploadMetadata) {
	e.Set("metadata", metadata)
}

func (e StorageObjectUploadedEvent) ObjectMetadata() *core.UploadMetadata {
	return e.Get("metadata").(*core.UploadMetadata)
}

func FireStorageObjectUploadedEvent(ctx core.Context, metadata *core.UploadMetadata) error {
	evt, err := getEvent(ctx, EVENT_STORAGE_OBJECT_UPLOADED)
	if err != nil {
		return err
	}

	configEvt, err := assertEventType[*StorageObjectUploadedEvent](evt, EVENT_STORAGE_OBJECT_UPLOADED)
	if err != nil {
		return err
	}

	configEvt.SetObjectMetadata(metadata)

	err = ctx.Event().FireEvent(configEvt)
	if err != nil {
		return err
	}

	return nil
}
