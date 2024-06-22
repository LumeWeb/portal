package event

import "go.lumeweb.com/portal/core"

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
