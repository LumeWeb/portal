package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
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
			pinService.metadata = core.GetService[core.MetadataService](ctx, core.METADATA_SERVICE)
			return nil
		}),
	)

	return pinService, opts, nil
}
func (p PinServiceDefault) AccountPins(id uint, createdAfter uint64) ([]models.Pin, error) {
	var pins []models.Pin

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&models.Pin{}).
			Preload("Upload"). // Preload the related Upload for each Pin
			Where(&models.Pin{UserID: id}).
			Where("created_at > ?", createdAfter).
			Order("created_at desc").
			Find(&pins)
	}); err != nil {
		return nil, core.NewAccountError(core.ErrKeyPinsRetrievalFailed, err)
	}

	return pins, nil
}

func (p PinServiceDefault) DeletePinByHash(hash core.StorageHash, userId uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash.Multihash()}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&models.Upload{}).Where(&uploadQuery).Select("id").First(&uploadID)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	// Delete pins with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadID, UserID: userId}

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&pinQuery).Delete(&models.Pin{})

	}); err != nil {
		return err
	}

	return nil
}

func (p PinServiceDefault) PinByHash(hash core.StorageHash, userId uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash.Multihash()}

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&uploadQuery).Where(&uploadQuery).First(&uploadQuery)
	}); err != nil {
		return err
	}

	return p.PinByID(uploadQuery.ID, userId)
}

func (p PinServiceDefault) PinByID(uploadId uint, userId uint) error {
	var rowsAffected int64

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadId, UserID: userId}).First(&models.Pin{})
		rowsAffected = tx.RowsAffected

		return tx
	}); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	if rowsAffected > 0 {
		return nil
	}

	// Create a pin with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadId, UserID: userId}

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(&pinQuery)
	}); err != nil {
		return err
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
	if err = db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&models.Pin{}).Where(&models.Pin{UploadID: upload.ID, UserID: userId}).First(&pin)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
