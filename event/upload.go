package event

import "go.lumeweb.com/portal/core"

const EVENT_UPLOAD_COMPLETED = "upload.completed"

type UploadCompletedEvent struct {
	UserID   *uint
	UploadID uint
	Bytes    uint64
	IP       string
}

func NewUploadCompletedEvent(uploadID uint, bytes uint64, ip string, userId *uint) *UploadCompletedEvent {
	return &UploadCompletedEvent{
		UserID:   userId,
		UploadID: uploadID,
		Bytes:    bytes,
		IP:       ip,
	}
}

// OnUploadCompleted registers a handler to run when an upload is completed.
// This is a convenience wrapper around Listen for the EVENT_UPLOAD_COMPLETED event.
func OnUploadCompleted(ctx core.Context, handler func(uint, uint64, string, *uint) error, priority ...int) {
	core.Listen[UploadCompletedEvent](ctx, EVENT_UPLOAD_COMPLETED, func(e *core.CoreEvent[UploadCompletedEvent]) error {
		return handler(e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID)
	}, priority...)
}
