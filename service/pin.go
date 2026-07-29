package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	pinMetrics "go.lumeweb.com/portal/service/internal/pin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.PinService = (*PinServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.PIN_SERVICE,
		Factory: NewPinService,
		Depends: []string{core.UPLOAD_SERVICE},
		Metrics: pinMetrics.GetCollectors(),
	})
}

type PinServiceDefault struct {
	*core.BaseComponent
	metadata core.UploadService
	models   map[string]data_models.PinDataModel
	mutex    *sync.RWMutex
}

func (p *PinServiceDefault) RegisterPinModel(protocol string, model data_models.PinDataModel) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.models[protocol] = model
	p.Logger().Debug("Registered pin model", zap.String("protocol", protocol))
}

func (p *PinServiceDefault) GetPinModel(protocol string) (data_models.PinDataModel, bool) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	model, ok := p.models[protocol]
	return model, ok
}

func (p *PinServiceDefault) CreatePinModel(protocol string) (data_models.PinDataModel, error) {
	model, ok := p.GetPinModel(protocol)
	if !ok {
		return nil, fmt.Errorf("no model registered for protocol: %s", protocol)
	}
	return model.NewInstance().(data_models.PinDataModel), nil
}

func NewPinService() (core.Service, []core.ContextBuilderOption, error) {
	pinService := &PinServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			pinService.metadata = core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
			pinService.mutex = &sync.RWMutex{}
			pinService.models = make(map[string]data_models.PinDataModel)
			return nil
		}),
	)

	return pinService, opts, nil
}

func (p *PinServiceDefault) ID() string {
	return core.PIN_SERVICE
}

func (p *PinServiceDefault) AllAccountPins(ctx context.Context, id uint) ([]*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.AllAccountPins")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpListAccount),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpListAccount),
		func() ([]*models.Pin, error) {
			var pins []*models.Pin
			if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("user_id = ?", id).
					Preload("Upload").
					Find(&pins)
			}); err != nil {
				return nil, err
			}

			pinMetrics.PinsListed.WithLabelValues(pinMetrics.LabelOpListAccount).Inc()
			return pins, nil
		},
	)
}

func (p *PinServiceDefault) AccountPins(ctx context.Context, id uint, createdAfter uint64) ([]*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.AccountPins")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpListAccount),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpListAccount),
		func() ([]*models.Pin, error) {
			filter := core.PinFilter{
				UserID:       id,
				CreatedAfter: time.Unix(int64(createdAfter), 0),
				Limit:        1000, // Set an appropriate limit
			}

			var pins []*models.Pin
			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.
					Scopes(applyPinFilters(filter)).
					Preload("Upload").
					Find(&pins)
			})

			if err != nil {
				return nil, core.NewAccountError(core.ErrKeyPinsRetrievalFailed, err)
			}

			pinMetrics.PinsListed.WithLabelValues(pinMetrics.LabelOpListAccount).Inc()
			return pins, nil
		},
	)
}

func (p *PinServiceDefault) DeletePinByHash(ctx context.Context, hash core.StorageHash, userId uint) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.DeletePinByHash")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpDelete),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpDelete),
		func() error {
			pinRecord, err := p.QueryPin(ctx, nil, core.PinFilter{
				Hash:   hash,
				UserID: userId,
			})
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}

			err = p.DeletePin(ctx, pinRecord.ID)
			if err == nil {
				pinMetrics.PinsDeleted.WithLabelValues(pinMetrics.LabelOpDelete).Inc()
			}
			return err
		},
	)
}

func (p *PinServiceDefault) GetPinByHash(ctx context.Context, hash core.StorageHash, userId uint) (*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetPinByHash")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpGet),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpGet),
		func() (*models.Pin, error) {
			pinRecord, err := p.QueryPin(ctx, nil, core.PinFilter{
				Hash:   hash,
				UserID: userId,
			})
			if err == nil {
				pinMetrics.PinsQueried.WithLabelValues(pinMetrics.LabelOpGet).Inc()
			}
			return pinRecord, err
		},
	)
}

func (p *PinServiceDefault) PinByHash(ctx context.Context, hash core.StorageHash, userId uint, protocolData any) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.PinByHash")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpCreate),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpCreate),
		func() error {
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
		},
	)
}

func (p *PinServiceDefault) PinByID(ctx context.Context, uploadId uint, userId uint, protocolData any) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.PinByID")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpCreate),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpCreate),
		func() error {
			pin := &models.Pin{
				UserID:   userId,
				UploadID: uploadId,
			}

			_, err := p.CreatePin(ctx, pin, protocolData)
			return err
		},
	)
}

func (p *PinServiceDefault) UploadPinnedGlobal(ctx context.Context, hash core.StorageHash) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.UploadPinnedGlobal")
	defer span.End()

	return p.UploadPinnedByUser(ctx, hash, 0)
}

func (p *PinServiceDefault) UploadPinnedByUser(ctx context.Context, hash core.StorageHash, userId uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.UploadPinnedByUser")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpCheckUser),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpCheckUser),
		func() (bool, error) {
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

			pinMetrics.PinsChecked.WithLabelValues(pinMetrics.LabelOpCheckUser).Inc()
			return pin != nil, nil
		},
	)
}

func (p *PinServiceDefault) GetAllPinsByHash(ctx context.Context, hash core.StorageHash) ([]*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetAllPinsByHash")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpListHash),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpListHash),
		func() ([]*models.Pin, error) {
			var pins []*models.Pin
			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Model(&models.Pin{}).
					Joins("JOIN uploads ON uploads.id = pins.upload_id").
					Where("uploads.hash = ?", hash.Multihash()).
					Preload("Upload").
					Find(&pins)
			})

			if err != nil {
				return nil, err
			}

			pinMetrics.PinsListed.WithLabelValues(pinMetrics.LabelOpListHash).Inc()
			return pins, nil
		},
	)
}

func (p *PinServiceDefault) GetPinsByUploadID(ctx context.Context, uploadID uint) ([]*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetPinsByUploadID")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpListUpload),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpListUpload),
		func() ([]*models.Pin, error) {
			var pins []*models.Pin
			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadID}).Joins("Upload").Find(&pins)
			})

			if err != nil {
				return nil, err
			}

			pinMetrics.PinsListed.WithLabelValues(pinMetrics.LabelOpListUpload).Inc()
			return pins, nil
		},
	)
}

func (p *PinServiceDefault) CreatePin(ctx context.Context, pin *models.Pin, protocolData any) (*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.CreatePin")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpCreate),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpCreate),
		func() (*models.Pin, error) {
			if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Preload("Upload").FirstOrCreate(pin, &models.Pin{
					UploadID: pin.UploadID,
					UserID:   pin.UserID,
				})
			}); err != nil {
				return nil, err
			}

			if !core.ProtocolHasPinHandler(pin.Upload.Protocol) {
				p.Logger().Panic("protocol does not have a pin handler", zap.String("protocol", pin.Upload.Protocol))
			}

			handler := core.GetProtocolPinHandler(pin.Upload.Protocol)

			if protocolData == nil {
				protocolData = handler.GetProtocolPinModel()
			}

			if err := handler.CreateProtocolPin(ctx, pin.ID, protocolData); err != nil {
				return nil, err
			}

			pinMetrics.PinsCreated.WithLabelValues(pinMetrics.LabelOpCreate).Inc()
			return pin, nil
		},
	)
}

func (p *PinServiceDefault) UpdatePin(ctx context.Context, pin *models.Pin) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.UpdatePin")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpUpdate),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpUpdate),
		func() error {
			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Save(pin)
			})
			if err == nil {
				pinMetrics.PinsUpdated.WithLabelValues(pinMetrics.LabelOpUpdate).Inc()
			}
			return err
		},
	)
}

// GetPin retrieves a pin by ID
func (p *PinServiceDefault) GetPin(ctx context.Context, id uint) (*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetPin")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpGet),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpGet),
		func() (*models.Pin, error) {
			var pin models.Pin
			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Preload("Upload").First(&pin, id)
			})
			if err != nil {
				return nil, err
			}
			pinMetrics.PinsQueried.WithLabelValues(pinMetrics.LabelOpGet).Inc()
			return &pin, nil
		},
	)
}

// DeletePin deletes a pin by ID
func (p *PinServiceDefault) DeletePin(ctx context.Context, id uint) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.DeletePin")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpDelete),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpDelete),
		func() error {
			pinRecord, err := p.GetPin(ctx, id)
			if err != nil {
				return err
			}

			if !core.ProtocolHasPinHandler(pinRecord.Upload.Protocol) {
				p.Logger().Panic("protocol does not have a pin handler", zap.String("protocol", pinRecord.Upload.Protocol))
			}

			err = db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Delete(&models.Pin{}, id)
			})

			if err != nil {
				return err
			}

			if err = core.GetProtocolPinHandler(pinRecord.Upload.Protocol).DeleteProtocolPin(ctx, id); err != nil {
				return err
			}

			pinMetrics.PinsDeleted.WithLabelValues(pinMetrics.LabelOpDelete).Inc()
			return nil
		},
	)
}

func (p *PinServiceDefault) QueryPin(ctx context.Context, query interface{}, filter core.PinFilter) (*models.Pin, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.QueryPin")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpQuery),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpQuery),
		func() (*models.Pin, error) {
			var pin models.Pin

			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				tx = tx.Preload("Upload")
				if query != nil {
					tx = tx.Where(query)
				}

				return tx.Scopes(applyPinFilters(filter)).First(&pin)
			})

			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return nil, err
				}
				return nil, err
			}

			pinMetrics.PinsQueried.WithLabelValues(pinMetrics.LabelOpQuery).Inc()
			return &pin, nil
		},
	)
}

// UpdateProtocolPin updates the protocol-specific data for a pin
func (p *PinServiceDefault) UpdateProtocolPin(ctx context.Context, id uint, protocolData any) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.UpdateProtocolPin")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpUpdate),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpUpdate),
		func() error {
			pinRecord, err := p.GetPin(ctx, id)
			if err != nil {
				return err
			}

			if !core.ProtocolHasPinHandler(pinRecord.Upload.Protocol) {
				p.Logger().Panic("protocol does not have a pin handler", zap.String("protocol", pinRecord.Upload.Protocol))
			}

			handler := core.GetProtocolPinHandler(pinRecord.Upload.Protocol)

			if protocolData == nil {
				protocolData = handler.GetProtocolPinModel()
			}

			err = handler.UpdateProtocolPin(ctx, id, protocolData)
			if err == nil {
				pinMetrics.PinsUpdated.WithLabelValues(pinMetrics.LabelOpUpdate).Inc()
			}
			return err
		},
	)
}

// GetProtocolPin retrieves the protocol-specific data for a pin
func (p *PinServiceDefault) GetProtocolPin(ctx context.Context, id uint) (any, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetProtocolPin")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpGetProtocol),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpGetProtocol),
		func() (any, error) {
			pinRecord, err := p.GetPin(ctx, id)
			if err != nil {
				return nil, err
			}

			if !core.ProtocolHasPinHandler(pinRecord.Upload.Protocol) {
				p.Logger().Panic("protocol does not have a pin handler", zap.String("protocol", pinRecord.Upload.Protocol))
			}

			result, err := core.GetProtocolPinHandler(pinRecord.Upload.Protocol).GetProtocolPin(ctx, p.DB().Preload("Pin"), id)
			if err == nil {
				pinMetrics.ProtocolPinsQueried.WithLabelValues(pinMetrics.LabelOpGetProtocol).Inc()
			}
			return result, err
		},
	)
}

func (p *PinServiceDefault) QueryProtocolPin(ctx context.Context, protocol string, query any, filter core.PinFilter) (any, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.QueryProtocolPin")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpQueryProtocol),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpQueryProtocol),
		func() (any, error) {
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

			err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				tx = tx.Model(result)
				tx = handler.QueryProtocolPin(ctx, query)

				if tx == nil {
					p.Logger().Panic("QueryProtocolPin returned nil")
				}

				tx = tx.Joins("Pin")

				return tx.Scopes(applyProtocolPinFilters(filter)).First(result)
			})

			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return nil, nil
				}
				return nil, err
			}

			pinMetrics.ProtocolPinsQueried.WithLabelValues(pinMetrics.LabelOpQueryProtocol).Inc()
			return result, nil
		},
	)
}

func (p *PinServiceDefault) GetPinData(ctx context.Context, pin *models.Pin) (interface{}, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetPinData")
	defer span.End()

	return core.MetricTrackResult(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpGetData),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpGetData),
		func() (interface{}, error) {
			// Get model for this protocol
			model, err := p.CreatePinModel(pin.Upload.Protocol)
			if err != nil {
				return nil, err
			}

			// Query the database
			if err = db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("pin_id = ?", pin.ID).First(model)
			}); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, nil // No data found, but not an error
				}
				return nil, err
			}

			pinMetrics.PinDataQueried.WithLabelValues(pinMetrics.LabelOpGetData).Inc()
			return model, nil
		},
	)
}

func (p *PinServiceDefault) UpdatePinData(ctx context.Context, pin *models.Pin, data interface{}) error {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.UpdatePinData")
	defer span.End()

	return core.MetricTrack(
		pinMetrics.PinDuration.WithLabelValues(pinMetrics.LabelOpUpdateData),
		pinMetrics.PinFailed.WithLabelValues(pinMetrics.LabelOpUpdateData),
		func() error {
			// Get model for this protocol
			model, err := p.CreatePinModel(pin.Upload.Protocol)
			if err != nil {
				return err
			}

			// Copy data from provided struct to model
			dataBytes, err := json.Marshal(data)
			if err != nil {
				return fmt.Errorf("failed to marshal  %w", err)
			}

			if err := json.Unmarshal(dataBytes, model); err != nil {
				return fmt.Errorf("failed to unmarshal data into model: %w", err)
			}

			// Set pin ID
			model.SetPinID(pin.ID)
			model.SetPin(pin)

			// Validate
			if err := model.Validate(); err != nil {
				return fmt.Errorf("data validation failed: %w", err)
			}

			// Store in database
			err = db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("pin_id = ?", pin.ID).First(model)
			})

			pinMetrics.PinDataUpdated.WithLabelValues(pinMetrics.LabelOpUpdateData).Inc()
			return err
		},
	)
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

func (p *PinServiceDefault) GetPinStats(ctx context.Context) ([]core.ProtocolPinStat, error) {
	ctx, span := core.TraceMethod(ctx, "PinServiceDefault.GetPinStats")
	defer span.End()

	var stats []core.ProtocolPinStat

	if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Table("pins p").
			Joins("JOIN uploads u ON p.upload_id = u.id").
			Select("u.protocol, COUNT(p.id) as total_pins").
			Where("u.deleted_at IS NULL").
			Group("u.protocol").
			Scan(&stats)
	}); err != nil {
		return nil, fmt.Errorf("failed to get pin stats: %w", err)
	}

	return stats, nil
}
