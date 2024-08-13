package event

import "go.lumeweb.com/portal/core"

const EVENT_DOWNLOAD_COMPLETED = "download.completed"

type DownloadCompletedEvent struct {
	core.Event
}

func (e *DownloadCompletedEvent) SetUploadID(uploadID uint) {
	e.Set("upload_id", uploadID)
}

func (e DownloadCompletedEvent) UploadID() uint {
	return e.Get("upload_id").(uint)
}

func (e *DownloadCompletedEvent) SetBytes(size uint64) {
	e.Set("bytes", size)
}

func (e DownloadCompletedEvent) Bytes() uint64 {
	return e.Get("bytes").(uint64)
}

func (e DownloadCompletedEvent) IP() string {
	return e.Get("ip").(string)
}

func (e *DownloadCompletedEvent) SetIP(ip string) {
	e.Set("ip", ip)
}

func FireDownloadCompletedEvent(ctx core.Context, uploadID uint, bytes uint64, ip string) error {
	return Fire[*DownloadCompletedEvent](ctx, EVENT_DOWNLOAD_COMPLETED, func(evt *DownloadCompletedEvent) error {
		evt.SetUploadID(uploadID)
		evt.SetBytes(bytes)
		evt.SetIP(ip)
		return nil
	})
}

func FireDownloadCompletedEventAsync(ctx core.Context, uploadID uint, bytes uint64, ip string) error {
	return FireAsync[*DownloadCompletedEvent](ctx, EVENT_DOWNLOAD_COMPLETED, func(evt *DownloadCompletedEvent) error {
		evt.SetUploadID(uploadID)
		evt.SetBytes(bytes)
		evt.SetIP(ip)
		return nil
	})
}
