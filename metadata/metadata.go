package metadata

import (
	"context"
	"errors"
	"time"

	"github.com/LumeWeb/portal/db/models"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

var _ MetadataService = (*MetadataServiceDefault)(nil)

type UploadMetadata struct {
	ID         uint      `json:"upload_id"`
	UserID     uint      `json:"user_id"`
	Hash       []byte    `json:"hash"`
	MimeType   string    `json:"mime_type"`
	Protocol   string    `json:"protocol"`
	UploaderIP string    `json:"uploader_ip"`
	Size       uint64    `json:"size"`
	Created    time.Time `json:"created"`
}

func (u UploadMetadata) IsEmpty() bool {
	if u.UserID != 0 || u.MimeType != "" || u.Protocol != "" || u.UploaderIP != "" || u.Size != 0 {
		return false
	}

	if !u.Created.IsZero() {
		return false
	}

	if len(u.Hash) != 0 {
		return false
	}

	return true
}

var Module = fx.Module("metadata",
	fx.Provide(
		fx.Annotate(
			NewMetadataService,
			fx.As(new(MetadataService)),
		),
	),
)

type MetadataService interface {
	SaveUpload(ctx context.Context, metadata UploadMetadata, skipExisting bool) error
	GetUpload(ctx context.Context, objectHash []byte) (UploadMetadata, error)
	DeleteUpload(ctx context.Context, objectHash []byte) error
}

type MetadataServiceDefault struct {
	db *gorm.DB
}

type MetadataServiceParams struct {
	fx.In
	Db *gorm.DB
}

func NewMetadataService(params MetadataServiceParams) *MetadataServiceDefault {
	return &MetadataServiceDefault{
		db: params.Db,
	}
}

func (m *MetadataServiceDefault) SaveUpload(ctx context.Context, metadata UploadMetadata, skipExisting bool) error {
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

func (m *MetadataServiceDefault) createUpload(ctx context.Context, metadata UploadMetadata) error {
	upload := models.Upload{
		UserID:     metadata.UserID,
		Hash:       metadata.Hash,
		MimeType:   metadata.MimeType,
		Protocol:   metadata.Protocol,
		UploaderIP: metadata.UploaderIP,
		Size:       metadata.Size,
	}

	return m.db.WithContext(ctx).Create(&upload).Error
}

func (m *MetadataServiceDefault) GetUpload(ctx context.Context, objectHash []byte) (UploadMetadata, error) {
	var upload models.Upload

	upload.Hash = objectHash

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		return UploadMetadata{}, ret.Error
	}

	return UploadMetadata{
		ID:         upload.ID,
		UserID:     upload.UserID,
		Hash:       upload.Hash,
		MimeType:   upload.MimeType,
		Protocol:   upload.Protocol,
		UploaderIP: upload.UploaderIP,
		Size:       upload.Size,
	}, nil
}

func (m *MetadataServiceDefault) DeleteUpload(ctx context.Context, objectHash []byte) error {
	var upload models.Upload

	upload.Hash = objectHash

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		return ret.Error
	}

	return m.db.Delete(&upload).Error
}
