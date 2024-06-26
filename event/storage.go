package event

import (
	"fmt"
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
	evt, ok := ctx.Event().GetEvent(EVENT_STORAGE_OBJECT_UPLOADED)

	if !ok {
		return fmt.Errorf("event %s not found", EVENT_STORAGE_OBJECT_UPLOADED)
	}

	evt.(*StorageObjectUploadedEvent).SetObjectMetadata(metadata)

	err := ctx.Event().FireEvent(evt)
	if err != nil {
		return err
	}

	return nil
}
