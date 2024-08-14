package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
)

const TUS_SERVICE = "tus"

type TUSService interface {
	UploadExists(ctx context.Context, id string) (bool, *models.TUSRequest)
	UploadHashExists(ctx context.Context, hash StorageHash) (bool, *models.TUSRequest)
	Uploads(ctx context.Context, uploaderID uint) ([]*models.TUSRequest, error)
	CreateUpload(ctx context.Context, hash StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol StorageProtocol, mimeType string) (*models.TUSRequest, error)
	UploadProgress(ctx context.Context, uploadID string) error
	UploadCompleted(ctx context.Context, uploadID string) error
	DeleteUpload(ctx context.Context, uploadID string) error

	Service
}
