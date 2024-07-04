package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.PinService = (*PinServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.PIN_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewPinService()
		},
		Depends: []string{core.METADATA_SERVICE},
	})
}

type PinServiceDefault struct {
	ctx      core.Context
	config   config.Manager
	db       *gorm.DB
	metadata core.MetadataService
}

func NewPinService() (*PinServiceDefault, []core.ContextBuilderOption, error) {
	pinService := &PinServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			pinService.ctx = ctx
			pinService.config = ctx.Config()
			pinService.db = ctx.DB()
			pinService.metadata = ctx.Service(core.METADATA_SERVICE).(core.MetadataService)
			return nil
		}),
	)

	return pinService, opts, nil
}
func (p PinServiceDefault) AccountPins(id uint, createdAfter uint64) ([]models.Pin, error) {
	var pins []models.Pin

	result := p.db.Model(&models.Pin{}).
		Preload("Upload"). // Preload the related Upload for each Pin
		Where(&models.Pin{UserID: id}).
		Where("created_at > ?", createdAfter).
		Order("created_at desc").
		Find(&pins)

	if result.Error != nil {
		return nil, core.NewAccountError(core.ErrKeyPinsRetrievalFailed, result.Error)
	}

	return pins, nil
}

func (p PinServiceDefault) DeletePinByHash(hash []byte, userId uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	result := p.db.
		Model(&models.Upload{}).
		Where(&uploadQuery).
		Select("id").
		First(&uploadID)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// No record found, nothing to delete
			return nil
		}
		return result.Error
	}

	// Delete pins with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadID, UserID: userId}
	result = p.db.
		Where(&pinQuery).
		Delete(&models.Pin{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (p PinServiceDefault) PinByHash(hash []byte, userId uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	result := p.db.
		Model(&uploadQuery).
		Where(&uploadQuery).
		First(&uploadQuery)

	if result.Error != nil {
		return result.Error
	}

	return p.PinByID(uploadQuery.ID, userId)
}

func (p PinServiceDefault) PinByID(uploadId uint, userId uint) error {
	result := p.db.Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadId, UserID: userId}).First(&models.Pin{})

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}

	if result.RowsAffected > 0 {
		return nil
	}

	// Create a pin with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadId, UserID: userId}
	result = p.db.Create(&pinQuery)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (p PinServiceDefault) UploadPinnedGlobal(hash core.StorageHash) (bool, error) {
	return p.UploadPinnedByUser(hash, 0)
}

func (p PinServiceDefault) UploadPinnedByUser(hash core.StorageHash, userId uint) (bool, error) {
	upload, err := p.metadata.GetUpload(context.Background(), hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	var pin models.Pin
	result := p.db.Model(&models.Pin{}).Where(&models.Pin{UploadID: upload.ID, UserID: userId}).First(&pin)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil
		}

		return false, result.Error
	}

	return true, nil
}
