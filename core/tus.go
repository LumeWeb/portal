package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
)

const TUS_SERVICE = "tus"

type TUSService interface {
	UploadExists(ctx context.Context, protocol StorageProtocol, id string) (bool, *models.TUSRequest)
	UploadHashExists(ctx context.Context, protocol StorageProtocol, hash StorageHash) (bool, *models.TUSRequest)
	Uploads(ctx context.Context, protocol StorageProtocol, uploaderID uint) ([]*models.TUSRequest, error)
	CreateUpload(ctx context.Context, hash StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol StorageProtocol) (*models.TUSRequest, error)
	UploadProgress(ctx context.Context, protocol StorageProtocol, uploadID string) error
	UploadProcessing(ctx context.Context, protocol StorageProtocol, uploadID string) error
	UploadCompleted(ctx context.Context, protocol StorageProtocol, uploadID string) error
	DeleteUpload(ctx context.Context, protocol StorageProtocol, uploadID string) error
	SetHash(ctx context.Context, protocol StorageProtocol, uploadID string, hash StorageHash) error

	Service
}
