package core

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/db/models"
)

const UPLOAD_SERVICE = "upload"

var (
	ErrUploadNotFound = errors.New("upload not found")
)

// ProtocolUploadStat represents per-protocol aggregate upload statistics.
type ProtocolUploadStat struct {
	Protocol          string
	TotalUploads      uint64
	TotalStorageBytes uint64
}

type UploadService interface {
	SaveUpload(ctx context.Context, upload *models.Upload) error
	GetUpload(ctx context.Context, objectHash StorageHash) (*models.Upload, error)
	DeleteUpload(ctx context.Context, objectHash StorageHash) error
	GetAllUploads(ctx context.Context) ([]*models.Upload, error)
	GetUploadByID(ctx context.Context, uploadID uint) (*models.Upload, error)

	// GetUploadStats returns per-protocol aggregate upload statistics.
	// Groups by uploads.protocol, counts distinct uploads, and sums uploads.size.
	// Soft-deleted uploads are excluded by default GORM scope.
	GetUploadStats(ctx context.Context) ([]ProtocolUploadStat, error)

	Service
}
