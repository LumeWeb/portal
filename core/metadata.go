package core

import (
	"context"
	"time"
)

type UploadMetadata struct {
	ID         uint      `json:"upload_id"`
	UserID     uint      `json:"user_id"`
	Hash       []byte    `json:"hash"`
	MimeType   string    `json:"mime_type"`
	Protocol   string    `json:"protocol"`
	UploaderIP string    `json:"uploader_ip"`
	Size       uint64    `json:"size"`
	Created    time.Time `json:"created"`
}

func (u UploadMetadata) IsEmpty() bool {
	if u.UserID != 0 || u.MimeType != "" || u.Protocol != "" || u.UploaderIP != "" || u.Size != 0 {
		return false
	}

	if !u.Created.IsZero() {
		return false
	}

	if len(u.Hash) != 0 {
		return false
	}

	return true
}

type MetadataService interface {
	SaveUpload(ctx context.Context, metadata UploadMetadata, skipExisting bool) error
	GetUpload(ctx context.Context, objectHash []byte) (UploadMetadata, error)
	DeleteUpload(ctx context.Context, objectHash []byte) error
	GetAllUploads(ctx context.Context) ([]UploadMetadata, error)
}
