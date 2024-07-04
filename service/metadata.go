package service

import (
	"context"
	"errors"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
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

func (m *MetadataServiceDefault) SaveUpload(ctx context.Context, metadata core.UploadMetadata, skipExisting bool) error {
	var upload models.Upload

	upload.Hash = metadata.Hash

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return m.createUpload(ctx, metadata)
		}
		return ret.Error
	}

	if skipExisting {
		return nil
	}

	changed := false

	if upload.UserID != metadata.UserID {
		upload.UserID = metadata.UserID
		changed = true
	}

	if upload.MimeType != metadata.MimeType {
		upload.MimeType = metadata.MimeType
		changed = true
	}

	if upload.Protocol != metadata.Protocol {
		upload.Protocol = metadata.Protocol
		changed = true
	}

	if upload.UploaderIP != metadata.UploaderIP {
		upload.UploaderIP = metadata.UploaderIP
		changed = true
	}

	if upload.Size != metadata.Size {
		upload.Size = metadata.Size
		changed = true
	}

	if changed {
		return m.db.Updates(&upload).Error
	}

	return nil
}

func (m *MetadataServiceDefault) createUpload(ctx context.Context, metadata core.UploadMetadata) error {
	upload := models.Upload{
		UserID:     metadata.UserID,
		Hash:       metadata.Hash,
		HashType:   metadata.HashType,
		MimeType:   metadata.MimeType,
		Protocol:   metadata.Protocol,
		UploaderIP: metadata.UploaderIP,
		Size:       metadata.Size,
	}

	return m.db.WithContext(ctx).Create(&upload).Error
}

func (m *MetadataServiceDefault) GetUpload(ctx context.Context, objectHash core.StorageHash) (core.UploadMetadata, error) {
	var upload models.Upload

	decoded, err := mh.Decode(objectHash.Multihash())
	if err != nil {
		return core.UploadMetadata{}, err
	}

	upload.Hash = decoded.Digest

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		return core.UploadMetadata{}, ret.Error
	}

	return m.uploadToMetadata(upload), nil
}

func (m *MetadataServiceDefault) DeleteUpload(ctx context.Context, objectHash core.StorageHash) error {
	var upload models.Upload

	decoded, err := mh.Decode(objectHash.Multihash())
	if err != nil {
		return err
	}

	upload.Hash = decoded.Digest

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		return ret.Error
	}

	return m.db.Delete(&upload).Error
}

func (m *MetadataServiceDefault) GetAllUploads(ctx context.Context) ([]core.UploadMetadata, error) {
	var uploads []models.Upload

	ret := m.db.WithContext(ctx).Find(&uploads)

	if ret.Error != nil {
		return nil, ret.Error
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
