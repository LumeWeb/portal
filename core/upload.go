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

type UploadService interface {
	SaveUpload(ctx context.Context, upload *models.Upload) error
	GetUpload(ctx context.Context, objectHash StorageHash) (*models.Upload, error)
	DeleteUpload(ctx context.Context, objectHash StorageHash) error
	GetAllUploads(ctx context.Context) ([]*models.Upload, error)
	GetUploadByID(ctx context.Context, uploadID uint) (*models.Upload, error)

	Service
}
