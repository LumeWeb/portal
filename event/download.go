package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_DOWNLOAD_COMPLETED = "download.completed"

// DownloadCompletedEvent represents a completed download event.
// UserID is optional - nil indicates an anonymous download (no associated user).
type DownloadCompletedEvent struct {
	UserID   *uint // Optional user ID; nil means anonymous download
	UploadID uint
	Bytes    uint64
	IP       string
	Ctx      context.Context
}

func NewDownloadCompletedEvent(uploadID uint, bytes uint64, ip string, userID *uint, eventCtx context.Context) *DownloadCompletedEvent {
	return &DownloadCompletedEvent{
		UserID:   userID,
		UploadID: uploadID,
		Bytes:    bytes,
		IP:       ip,
		Ctx:      eventCtx,
	}
}

// OnDownloadCompleted registers a handler to run when a download is completed.
// This is a convenience wrapper around Listen for the EVENT_DOWNLOAD_COMPLETED event.
func OnDownloadCompleted(ctx core.Context, handler func(uploadID uint, bytes uint64, ip string, userID *uint, eventCtx context.Context) error, priority ...int) {
	core.Listen[DownloadCompletedEvent](ctx, EVENT_DOWNLOAD_COMPLETED, func(e *core.CoreEvent[DownloadCompletedEvent]) error {
		return handler(e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID, e.Data.Ctx)
	}, priority...)
}
