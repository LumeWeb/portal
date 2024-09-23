package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
	"sync"
)

const UPLOAD_SERVICE = "upload"

var (
	uploadDataHandlers   = make(map[string]UploadDataHandler)
	uploadDataHandlersMu sync.RWMutex
)

type UploadDataHandler interface {
	CreateUploadData(ctx context.Context, tx *gorm.DB, id uint, data any) error
	GetUploadData(ctx context.Context, tx *gorm.DB, id uint) (any, error)
	UpdateUploadData(ctx context.Context, tx *gorm.DB, id uint, data any) error
	DeleteUploadData(ctx context.Context, tx *gorm.DB, id uint) error
	QueryUploadData(ctx context.Context, tx *gorm.DB, query any) *gorm.DB
	CompleteUploadData(ctx context.Context, tx *gorm.DB, id uint) error
	GetUploadDataModel() any
}

func RegisterUploadDataHandler(id string, handler UploadDataHandler) {
	uploadDataHandlersMu.Lock()
	defer uploadDataHandlersMu.Unlock()

	if _, ok := uploadDataHandlers[id]; ok {
		panic("upload data handler already registered: " + id)
	}

	uploadDataHandlers[id] = handler
}

func GetUploadDataHandler(id string) (UploadDataHandler, bool) {
	uploadDataHandlersMu.RLock()
	defer uploadDataHandlersMu.RUnlock()

	handler, ok := uploadDataHandlers[id]
	return handler, ok
}

type UploadService interface {
	SaveUpload(ctx context.Context, upload *models.Upload) error
	GetUpload(ctx context.Context, objectHash StorageHash) (*models.Upload, error)
	DeleteUpload(ctx context.Context, objectHash StorageHash) error
	GetAllUploads(ctx context.Context) ([]*models.Upload, error)
	GetUploadByID(ctx context.Context, uploadID uint) (*models.Upload, error)

	Service
}
