package core

import (
	"context"
	"time"
)

const METADATA_SERVICE = "metadata"

type UploadMetadata struct {
	ID         uint      `json:"upload_id"`
	UserID     uint      `json:"user_id"`
	Hash       []byte    `json:"hash"`
	HashType   uint64    `json:"hash_type"`
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
	SaveUpload(ctx context.Context, metadata UploadMetadata) error
	GetUpload(ctx context.Context, objectHash StorageHash) (UploadMetadata, error)
	DeleteUpload(ctx context.Context, objectHash StorageHash) error
	GetAllUploads(ctx context.Context) ([]UploadMetadata, error)

	Service
}
