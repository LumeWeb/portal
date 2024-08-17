package service

import (
	"context"
	"errors"
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"reflect"
	"time"
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
	logger   *core.Logger
	config   config.Manager
	db       *gorm.DB
	metadata core.MetadataService
}

func NewPinService() (*PinServiceDefault, []core.ContextBuilderOption, error) {
	pinService := &PinServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			pinService.ctx = ctx
			pinService.logger = ctx.ServiceLogger(pinService)
			pinService.config = ctx.Config()
			pinService.db = ctx.DB()
			pinService.metadata = core.GetService[core.MetadataService](ctx, core.METADATA_SERVICE)
			return nil
		}),
	)

	return pinService, opts, nil
}

func (p PinServiceDefault) ID() string {
	return core.PIN_SERVICE
}

func (p PinServiceDefault) AccountPins(id uint, createdAfter uint64) ([]models.Pin, error) {
	ctx := context.Background()
	filter := core.PinFilter{
		UserID:       id,
		CreatedAfter: time.Unix(int64(createdAfter), 0),
		Limit:        1000, // Set an appropriate limit
	}

	var pins []models.Pin
	err := p.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Scopes(applyPinFilters(filter)).
				Preload("Upload").
				Find(&pins)
		})
	})

	if err != nil {
		return nil, core.NewAccountError(core.ErrKeyPinsRetrievalFailed, err)
	}

	return pins, nil
}

func (p PinServiceDefault) DeletePinByHash(hash core.StorageHash, userId uint) error {
	ctx := context.Background()
	pin, err := p.QueryPin(ctx, nil, core.PinFilter{
		Hash:   hash,
		UserID: userId,
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	return p.DeletePin(ctx, pin.ID)
}

func (p PinServiceDefault) PinByHash(hash core.StorageHash, userId uint, protocolData any) error {
	ctx := context.Background()
	upload, err := p.metadata.GetUpload(ctx, hash)
	if err != nil {
		return err
	}

	pin := &models.Pin{
		UserID:   userId,
		UploadID: upload.ID,
	}

	_, err = p.CreatePin(ctx, pin, protocolData)
	return err
}

func (p PinServiceDefault) PinByID(uploadId uint, userId uint, protocolData any) error {
	ctx := context.Background()
	pin := &models.Pin{
		UserID:   userId,
		UploadID: uploadId,
	}

	_, err := p.CreatePin(ctx, pin, protocolData)
	return err
}

func (p PinServiceDefault) UploadPinnedGlobal(hash core.StorageHash) (bool, error) {
	return p.UploadPinnedByUser(hash, 0)
}

func (p PinServiceDefault) UploadPinnedByUser(hash core.StorageHash, userId uint) (bool, error) {
	ctx := context.Background()
	upload, err := p.metadata.GetUpload(ctx, hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	filter := core.PinFilter{UploadID: upload.ID}
	if userId != 0 {
		filter.UserID = userId
	}

	pin, err := p.QueryPin(ctx, nil, filter)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return pin != nil, nil
}

func (p *PinServiceDefault) CreatePin(ctx context.Context, pin *models.Pin, protocolData any) (*models.Pin, error) {
	if err := p.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Preload("Upload").FirstOrCreate(pin, &models.Pin{
				UploadID: pin.UploadID,
				UserID:   pin.UserID,
			})
		})
	}); err != nil {
		return nil, err
	}

	if !core.ProtocolHasPinHandler(pin.Upload.Protocol) {
		p.logger.Panic("protocol does not have a pin handler", zap.String("protocol", pin.Upload.Protocol))
	}

	handler := core.GetProtocolPinHandler(pin.Upload.Protocol)

	if protocolData == nil {
		protocolData = handler.GetProtocolPinModel()
	}

	if err := handler.CreateProtocolPin(ctx, pin.ID, protocolData); err != nil {
		return nil, err
	}

	return pin, nil
}

func (p PinServiceDefault) UpdatePin(ctx context.Context, pin *models.Pin) error {
	return p.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Save(pin)
		})
	})
}

// GetPin retrieves a pin by ID
func (p *PinServiceDefault) GetPin(ctx context.Context, id uint) (*models.Pin, error) {
	var pin models.Pin
	err := p.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Preload("Upload").First(&pin, id)
		})
	})
	if err != nil {
		return nil, err
	}
	return &pin, nil
}

// DeletePin deletes a pin by ID
func (p *PinServiceDefault) DeletePin(ctx context.Context, id uint) error {
	pin, err := p.GetPin(ctx, id)
	if err != nil {
		return err
	}

	if !core.ProtocolHasPinHandler(pin.Upload.Protocol) {
		p.logger.Panic("protocol does not have a pin handler", zap.String("protocol", pin.Upload.Protocol))
	}

	err = p.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Delete(&models.Pin{}, id)
		})
	})

	if err != nil {
		return err
	}

	if err = core.GetProtocolPinHandler(pin.Upload.Protocol).DeleteProtocolPin(ctx, id); err != nil {
		return err
	}

	return nil
}

func (p *PinServiceDefault) QueryPin(ctx context.Context, query interface{}, filter core.PinFilter) (*models.Pin, error) {
	var pin models.Pin

	err := p.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx = db.WithContext(ctx).Preload("Upload")
			if query != nil {
				tx = tx.Where(query)
			}

			return tx.Scopes(applyPinFilters(filter)).First(&pin)
		})
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, err
	}

	return &pin, nil
}

// UpdateProtocolPin updates the protocol-specific data for a pin
func (p *PinServiceDefault) UpdateProtocolPin(ctx context.Context, id uint, protocolData any) error {
	pin, err := p.GetPin(ctx, id)
	if err != nil {
		return err
	}

	if !core.ProtocolHasPinHandler(pin.Upload.Protocol) {
		p.logger.Panic("protocol does not have a pin handler", zap.String("protocol", pin.Upload.Protocol))
	}

	handler := core.GetProtocolPinHandler(pin.Upload.Protocol)

	if protocolData == nil {
		protocolData = handler.GetProtocolPinModel()
	}

	return handler.UpdateProtocolPin(ctx, id, protocolData)
}

// GetProtocolPin retrieves the protocol-specific data for a pin
func (p *PinServiceDefault) GetProtocolPin(ctx context.Context, id uint) (any, error) {
	pin, err := p.GetPin(ctx, id)
	if err != nil {
		return nil, err
	}

	if !core.ProtocolHasPinHandler(pin.Upload.Protocol) {
		p.logger.Panic("protocol does not have a pin handler", zap.String("protocol", pin.Upload.Protocol))
	}

	return core.GetProtocolPinHandler(pin.Upload.Protocol).GetProtocolPin(ctx, p.db.Preload("Pin"), id)
}

func (p *PinServiceDefault) QueryProtocolPin(ctx context.Context, protocol string, query any, filter core.PinFilter) (any, error) {
	if !core.ProtocolHasPinHandler(protocol) {
		return nil, fmt.Errorf("protocol %s does not have a data request handler", protocol)
	}

	handler := core.GetProtocolPinHandler(protocol)

	model := handler.GetProtocolPinModel()
	mt := reflect.TypeOf(model)

	if mt.Kind() == reflect.Ptr {
		mt = mt.Elem()
	}

	// Create a new instance of the model type
	result := reflect.New(mt).Interface()

	err := p.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx = db.WithContext(ctx).Model(result)
			tx = handler.QueryProtocolPin(ctx, query)

			if tx == nil {
				p.logger.Panic("QueryProtocolPin returned nil")
			}

			tx = tx.Joins("Pin")

			return tx.Scopes(applyProtocolPinFilters(filter)).First(result)
		})
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return result, nil
}

// Helper function to apply pin filters
func applyPinFilters(filter core.PinFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		// Join with uploads table if we need to filter by upload properties
		if filter.Hash != nil || filter.Protocol != "" {
			db = db.Joins("JOIN uploads ON uploads.id = pins.upload_id")
		}

		if filter.UploadID != 0 {
			db = db.Where("pins.upload_id = ?", filter.UploadID)
		}

		if filter.Hash != nil {
			db = db.Where("uploads.hash = ?", filter.Hash.Multihash())
		}

		if filter.UserID != 0 {
			db = db.Where("pins.user_id = ?", filter.UserID)
		}

		if !filter.CreatedAfter.IsZero() {
			db = db.Where("pins.created_at > ?", filter.CreatedAfter)
		}

		if filter.Protocol != "" {
			db = db.Where("uploads.protocol = ?", filter.Protocol)
		}

		if filter.Limit > 0 {
			db = db.Limit(filter.Limit)
		}

		if filter.Offset > 0 {
			db = db.Offset(filter.Offset)
		}

		// Always preload Upload data
		db = db.Preload("Upload")

		return db.Order("pins.created_at DESC")
	}
}

func applyProtocolPinFilters(filter core.PinFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		// Join with uploads table if we need to filter by upload properties
		if filter.Hash != nil || filter.Protocol != "" {
			db = db.Joins("Pin.Upload")
		}

		if filter.UploadID != 0 {
			db = db.Where("Pin.upload_id = ?", filter.UploadID)
		}

		if filter.Hash != nil {
			db = db.Where("Pin__Upload.hash = ?", filter.Hash.Multihash())
		}

		if filter.UserID != 0 {
			db = db.Where("Pin.user_id = ?", filter.UserID)
		}

		if !filter.CreatedAfter.IsZero() {
			db = db.Where("Pin.created_at > ?", filter.CreatedAfter)
		}

		if filter.Protocol != "" {
			db = db.Where("Pin__Upload.protocol = ?", filter.Protocol)
		}

		if filter.Limit > 0 {
			db = db.Limit(filter.Limit)
		}

		if filter.Offset > 0 {
			db = db.Offset(filter.Offset)
		}

		// Always preload Upload data
		db = db.Preload("Pin.Upload")

		return db.Order("Pin.created_at DESC")
	}
}
