package core

import (
	"context"
	"gorm.io/gorm"
	"sync"
)

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
