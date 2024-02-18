package metadata

import (
	"context"
	"errors"
	"time"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type UploadMetadata struct {
	UserID     uint      `json:"userId"`
	Hash       []byte    `json:"hash"`
	MimeType   string    `json:"mimeType"`
	Protocol   string    `json:"protocol"`
	UploaderIP string    `json:"uploaderIp"`
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
	SaveUpload(ctx context.Context, metadata UploadMetadata) error
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

func (m *MetadataServiceDefault) SaveUpload(ctx context.Context, metadata UploadMetadata) error {
	var upload models.Upload

	upload.Hash = metadata.Hash

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		if !errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return ret.Error
		}
	}

	if ret.RowsAffected > 0 {
		return nil
	}

	upload.UserID = metadata.UserID
	upload.MimeType = metadata.MimeType
	upload.Protocol = metadata.Protocol
	upload.UploaderIP = metadata.UploaderIP
	upload.Size = metadata.Size

	return m.db.Save(&metadata).Error
}

func (m *MetadataServiceDefault) GetUpload(ctx context.Context, objectHash []byte) (UploadMetadata, error) {
	var upload models.Upload

	upload.Hash = objectHash

	ret := m.db.WithContext(ctx).Model(&models.Upload{}).Where(&upload).First(&upload)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return UploadMetadata{}, ret.Error
		}
	}

	return UploadMetadata{
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
