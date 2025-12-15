package event

import "go.lumeweb.com/portal/core"

const EVENT_DOWNLOAD_COMPLETED = "download.completed"

type DownloadCompletedEvent struct {
	UserID   *uint
	UploadID uint
	Bytes    uint64
	IP       string
}

func NewDownloadCompletedEvent(uploadID uint, bytes uint64, ip string, userID *uint) *DownloadCompletedEvent {
	return &DownloadCompletedEvent{
		UserID:   userID,
		UploadID: uploadID,
		Bytes:    bytes,
		IP:       ip,
	}
}

// OnDownloadCompleted registers a handler to run when a download is completed.
// This is a convenience wrapper around Listen for the EVENT_DOWNLOAD_COMPLETED event.
func OnDownloadCompleted(ctx core.Context, handler func(uploadID uint, bytes uint64, ip string, userID *uint) error, priority ...int) {
	core.Listen[DownloadCompletedEvent](ctx, EVENT_DOWNLOAD_COMPLETED, func(e *core.CoreEvent[DownloadCompletedEvent]) error {
		return handler(e.Data.UploadID, e.Data.Bytes, e.Data.IP, e.Data.UserID)
	}, priority...)
}
