package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_DOWNLOAD_COMPLETED = "download.completed"

// DownloadCompletedEvent represents a completed download event.
// UserID is optional - nil indicates an anonymous download (no associated user).
// ReservationID is optional - if provided, a quota reservation was created for this download.
type DownloadCompletedEvent struct {
	Ctx            context.Context
	UserID         *uint // Optional user ID; nil means anonymous download
	ReservationID  *string // Optional reservation ID; if set, should be committed on success or released on failure
	UploadID       uint
	Bytes          uint64
	IP             string
	Successful     bool  // Indicates if the download completed successfully
}

func NewDownloadCompletedEvent(eventCtx context.Context, uploadID uint, bytes uint64, ip string, userID *uint, reservationID *string, successful bool) *DownloadCompletedEvent {
	return &DownloadCompletedEvent{
		Ctx:           eventCtx,
		UserID:        userID,
		ReservationID: reservationID,
		UploadID:      uploadID,
		Bytes:         bytes,
		IP:            ip,
		Successful:    successful,
	}
}

// OnDownloadCompleted registers a handler to run when a download is completed.
// This is a convenience wrapper around Listen for the EVENT_DOWNLOAD_COMPLETED event.
func OnDownloadCompleted(ctx core.Context, handler func(context.Context, uint, uint64, string, *uint, *string, bool) error, priority ...int) {
	core.Listen[DownloadCompletedEvent](ctx, EVENT_DOWNLOAD_COMPLETED, func(e *core.CoreEvent[DownloadCompletedEvent]) error {
		return handler(e.Data.Ctx, e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID, e.Data.ReservationID, e.Data.Successful)
	}, priority...)
}
