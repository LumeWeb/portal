package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.UploadService = (*UploadServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.UPLOAD_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewMetadataService()
		},
	})
}

type UploadServiceDefault struct {
	ctx core.Context
	db  *gorm.DB
}

func NewMetadataService() (*UploadServiceDefault, []core.ContextBuilderOption, error) {
	meta := &UploadServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			meta.ctx = ctx
			meta.db = ctx.DB()
			return nil
		}),
	)

	return meta, opts, nil
}

func (m *UploadServiceDefault) ID() string {
	return core.UPLOAD_SERVICE
}

func (m *UploadServiceDefault) SaveUpload(ctx context.Context, upload *models.Upload) error {
	return db.RetryableTransaction(m.ctx, m.db, func(tx *gorm.DB) *gorm.DB {
		existingUpload := &models.Upload{
			Hash:     upload.Hash,
			HashType: upload.HashType,
			Protocol: upload.Protocol,
		}

		err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Model(existingUpload).Where(existingUpload).First(existingUpload)
		})

		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			_ = tx.AddError(err)
			return tx
		}

		// If the record doesn't exist, create a new one
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(upload)
		}

		// Update fields if they are different and not empty
		if upload.UserID != 0 && upload.UserID != existingUpload.UserID {
			existingUpload.UserID = upload.UserID
		}
		if upload.MimeType != "" && upload.MimeType != existingUpload.MimeType {
			existingUpload.MimeType = upload.MimeType
		}
		if upload.UploaderIP != "" && upload.UploaderIP != existingUpload.UploaderIP {
			existingUpload.UploaderIP = upload.UploaderIP
		}
		if upload.Size != 0 && upload.Size != existingUpload.Size {
			existingUpload.Size = upload.Size
		}

		return tx.Save(existingUpload)
	})
}

func (m *UploadServiceDefault) GetUpload(ctx context.Context, objectHash core.StorageHash) (*models.Upload, error) {
	var upload models.Upload
	upload.Hash = objectHash.Multihash()

	if err := db.RetryableTransaction(m.ctx, m.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&upload).Where(&upload).First(&upload)
	}); err != nil {
		return nil, err
	}

	return &upload, nil
}

func (m *UploadServiceDefault) DeleteUpload(ctx context.Context, objectHash core.StorageHash) error {
	var upload models.Upload
	upload.Hash = objectHash.Multihash()

	if err := db.RetryableTransaction(m.ctx, m.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&upload).Where(&upload).First(&upload)
	}); err != nil {
		return err
	}

	return db.RetryableTransaction(m.ctx, m.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Delete(&upload)
	})
}

func (m *UploadServiceDefault) GetAllUploads(ctx context.Context) ([]*models.Upload, error) {
	var uploads []*models.Upload

	if err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Find(&uploads)
		})
	}); err != nil {
		return nil, err
	}

	return uploads, nil
}

func (m *UploadServiceDefault) GetUploadByID(ctx context.Context, uploadID uint) (*models.Upload, error) {
	var upload models.Upload
	upload.ID = uploadID

	if err := db.RetryableTransaction(m.ctx, m.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Upload{}).Where(&upload).First(&upload)
	}); err != nil {
		return nil, err
	}

	return &upload, nil
}
