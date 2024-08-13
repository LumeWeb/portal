package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.MetadataService = (*MetadataServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.METADATA_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewMetadataService()
		},
	})
}

type MetadataServiceDefault struct {
	ctx core.Context
	db  *gorm.DB
}

func NewMetadataService() (*MetadataServiceDefault, []core.ContextBuilderOption, error) {
	meta := &MetadataServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			meta.ctx = ctx
			meta.db = ctx.DB()
			return nil
		}),
	)

	return meta, opts, nil
}

func (m *MetadataServiceDefault) ID() string {
	return core.METADATA_SERVICE
}

func (m *MetadataServiceDefault) SaveUpload(ctx context.Context, metadata core.UploadMetadata) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		upload := &models.Upload{
			Hash:     metadata.Hash,
			HashType: metadata.HashType,
			Protocol: metadata.Protocol,
		}

		if err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Model(upload).Where(&upload).First(&upload)
		}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		update := false

		if metadata.UserID != 0 && upload.UserID != metadata.UserID {
			upload.UserID = metadata.UserID
			update = true
		}

		if metadata.MimeType != "" && upload.MimeType != metadata.MimeType {
			upload.MimeType = metadata.MimeType
			update = true
		}

		if metadata.UploaderIP != "" && upload.UploaderIP != metadata.UploaderIP {
			upload.UploaderIP = metadata.UploaderIP
			update = true
		}

		if metadata.Size != 0 && upload.Size != metadata.Size {
			upload.Size = metadata.Size
			update = true
		}

		if update {
			if err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
				return db.WithContext(ctx).Save(&upload)
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (m *MetadataServiceDefault) GetUpload(ctx context.Context, objectHash core.StorageHash) (core.UploadMetadata, error) {
	var upload models.Upload

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		upload.Hash = objectHash.Multihash()

		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Model(&upload).Where(&upload).First(&upload)
		})
	})

	if err != nil {
		return core.UploadMetadata{}, err
	}

	return m.uploadToMetadata(upload), nil
}

func (m *MetadataServiceDefault) DeleteUpload(ctx context.Context, objectHash core.StorageHash) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var upload models.Upload
		upload.Hash = objectHash.Multihash()

		if err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Model(&models.Upload{}).Where(&upload).First(&upload)
		}); err != nil {
			return err
		}

		if err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Delete(&upload)
		}); err != nil {
			return err
		}

		return nil
	})
}

func (m *MetadataServiceDefault) GetAllUploads(ctx context.Context) ([]core.UploadMetadata, error) {
	var uploads []models.Upload

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Find(&uploads)
		})
	})

	if err != nil {
		return nil, err
	}

	var metadata []core.UploadMetadata
	for _, upload := range uploads {
		metadata = append(metadata, m.uploadToMetadata(upload))
	}

	return metadata, nil
}

func (m *MetadataServiceDefault) uploadToMetadata(upload models.Upload) core.UploadMetadata {
	return core.UploadMetadata{
		ID:         upload.ID,
		UserID:     upload.UserID,
		Hash:       upload.Hash,
		HashType:   upload.HashType,
		MimeType:   upload.MimeType,
		Protocol:   upload.Protocol,
		UploaderIP: upload.UploaderIP,
		Size:       upload.Size,
		Created:    upload.CreatedAt,
	}
}
