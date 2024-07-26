package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/internal/tus"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

type TusHandlerConfig = tus.HandlerConfig
type TusHandler = tus.TusHandler
type TUSUploadCallbackHandler = tus.UploadCallbackHandler
type TUSUploadCreatedVerifyFunc = tus.UploadCreatedVerifyFunc

var _ core.TUSService = (*TUSServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.TUS_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewTUSService()
		},
	})
}

type TUSServiceDefault struct {
	ctx    core.Context
	db     *gorm.DB
	logger *core.Logger
}

func NewTUSService() (*TUSServiceDefault, []core.ContextBuilderOption, error) {
	storage := &TUSServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			storage.ctx = ctx
			storage.db = ctx.DB()
			storage.logger = ctx.Logger()
			return nil
		}),
	)

	return storage, opts, nil
}

func (t *TUSServiceDefault) UploadExists(ctx context.Context, id string) (bool, *models.TusUpload) {
	var upload models.TusUpload
	var exists bool
	err := t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			result := rtx.WithContext(ctx).Where(&models.TusUpload{UploadID: id}).First(&upload)
			exists = result.RowsAffected > 0
			return result
		})
	})
	if err != nil {
		return false, nil
	}
	return exists, &upload
}

func (t *TUSServiceDefault) UploadHashExists(ctx context.Context, hash core.StorageHash) (bool, *models.TusUpload) {
	var upload models.TusUpload
	var exists bool

	err := t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			result := rtx.WithContext(ctx).Where(&models.TusUpload{Hash: hash.Multihash()}).First(&upload)
			exists = result.RowsAffected > 0
			return result
		})
	})
	if err != nil {
		return false, nil
	}
	return exists, &upload
}

func (t *TUSServiceDefault) Uploads(ctx context.Context, uploaderID uint) ([]*models.TusUpload, error) {
	var uploads []*models.TusUpload
	err := t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			return rtx.WithContext(ctx).Where(&models.TusUpload{UploaderID: uploaderID}).Find(&uploads)
		})
	})
	if err != nil {
		return nil, err
	}
	return uploads, nil
}

func (t *TUSServiceDefault) CreateUpload(ctx context.Context, hash core.StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol core.StorageProtocol, mimeType string) (*models.TusUpload, error) {
	var hashBytes []byte

	if hash != nil {
		hashBytes = hash.Multihash()
	}

	upload := &models.TusUpload{
		Hash:       hashBytes,
		UploadID:   uploadID,
		UploaderID: uploaderID,
		UploaderIP: uploaderIP,
		Uploader:   models.User{},
		Protocol:   protocol.Name(),
		MimeType:   mimeType,
	}
	err := t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			return rtx.WithContext(ctx).Create(upload)
		})
	})
	if err != nil {
		return nil, err
	}
	return upload, nil
}

func (t *TUSServiceDefault) UploadProgress(ctx context.Context, uploadID string) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			result := rtx.WithContext(ctx).Model(&models.TusUpload{}).
				Where(&models.TusUpload{UploadID: uploadID}).
				Update("updated_at", time.Now())
			if result.RowsAffected == 0 {
				_ = result.AddError(errors.New("upload not found"))
			}
			return result
		})
	})
}

func (t *TUSServiceDefault) UploadCompleted(ctx context.Context, uploadID string) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			result := rtx.WithContext(ctx).Model(&models.TusUpload{}).
				Where(&models.TusUpload{UploadID: uploadID}).
				Update("completed", true)
			if result.RowsAffected == 0 {
				_ = result.AddError(errors.New("upload not found"))
			}
			return result
		})
	})
}

func (t *TUSServiceDefault) DeleteUpload(ctx context.Context, uploadID string) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(rtx *gorm.DB) *gorm.DB {
			return rtx.WithContext(ctx).Where(&models.TusUpload{UploadID: uploadID}).Delete(&models.TusUpload{})
		})
	})
}

func CreateTusHandler(ctx core.Context, config TusHandlerConfig) (*tus.TusHandler, error) {
	handler, err := tus.NewTusHandler(ctx, config)
	if err != nil {
		ctx.Logger().Error("Failed to create tus handler", zap.Error(err))
		return nil, err
	}

	return handler, nil
}

func TUSDefaultUploadCreatedHandler(ctx core.Context, verifyFunc TUSUploadCreatedVerifyFunc) TUSUploadCallbackHandler {
	return tus.DefaultUploadCreatedHandler(ctx, verifyFunc)
}

func TUSDefaultUploadProgressHandler(ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadProgressHandler(ctx)
}

func TUSDefaultUploadCompletedHandler(ctx core.Context, processHandler TUSUploadCallbackHandler) TUSUploadCallbackHandler {
	return tus.DefaultUploadCompletedHandler(ctx, processHandler)
}

func TUSDefaultUploadTerminatedHandler(ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadTerminatedHandler(ctx)
}
