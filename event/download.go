package event

const EVENT_DOWNLOAD_COMPLETED = "download.completed"

type DownloadCompletedEvent struct {
	UploadID uint
	Bytes    uint64
	IP       string
}

func NewDownloadCompletedEvent(uploadID uint, bytes uint64, ip string) *DownloadCompletedEvent {
	return &DownloadCompletedEvent{
		UploadID: uploadID,
		Bytes:    bytes,
		IP:       ip,
	}
}
