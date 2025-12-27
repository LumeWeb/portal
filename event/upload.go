package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_UPLOAD_COMPLETED = "upload.completed"

type UploadCompletedEvent struct {
	Ctx      context.Context
	UserID   *uint
	UploadID uint
	Bytes    uint64
	IP       string
}

func NewUploadCompletedEvent(eventCtx context.Context, uploadID uint, bytes uint64, ip string, userId *uint) *UploadCompletedEvent {
	return &UploadCompletedEvent{
		Ctx:      eventCtx,
		UserID:   userId,
		UploadID: uploadID,
		Bytes:    bytes,
		IP:       ip,
	}
}

// OnUploadCompleted registers a handler to run when an upload is completed.
// This is a convenience wrapper around Listen for the EVENT_UPLOAD_COMPLETED event.
func OnUploadCompleted(ctx core.Context, handler func(context.Context, uint, uint64, string, *uint) error, priority ...int) {
	core.Listen[UploadCompletedEvent](ctx, EVENT_UPLOAD_COMPLETED, func(e *core.CoreEvent[UploadCompletedEvent]) error {
		return handler(e.Data.Ctx, e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID)
	}, priority...)
}
