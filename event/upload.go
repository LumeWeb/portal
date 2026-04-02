package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_UPLOAD_COMPLETED = "upload.completed"

// UploadCompletedEvent represents a completed upload event.
// UserID is optional - nil indicates an anonymous upload (no associated user).
// ReservationID is optional - if provided, a quota reservation was created for this upload.
type UploadCompletedEvent struct {
	Ctx            context.Context
	UserID         *uint // Optional user ID; nil means anonymous upload
	ReservationID  *string // Optional reservation ID; if set, should be committed on success or released on failure
	UploadID       uint
	Bytes          uint64
	IP             string
	Successful     bool  // Indicates if the upload completed successfully
}

func NewUploadCompletedEvent(eventCtx context.Context, uploadID uint, bytes uint64, ip string, userId *uint, reservationID *string, successful bool) *UploadCompletedEvent {
	return &UploadCompletedEvent{
		Ctx:           eventCtx,
		UserID:        userId,
		ReservationID: reservationID,
		UploadID:      uploadID,
		Bytes:         bytes,
		IP:            ip,
		Successful:    successful,
	}
}

// OnUploadCompleted registers a handler to run when an upload is completed.
// This is a convenience wrapper around Listen for the EVENT_UPLOAD_COMPLETED event.
func OnUploadCompleted(ctx core.Context, handler func(context.Context, uint, uint64, string, *uint, *string, bool) error, priority ...int) {
	core.Listen[UploadCompletedEvent](ctx, EVENT_UPLOAD_COMPLETED, func(e *core.CoreEvent[UploadCompletedEvent]) error {
		return handler(e.Data.Ctx, e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID, e.Data.ReservationID, e.Data.Successful)
	}, priority...)
}
