package core

import (
	"context"
	"time"

	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
)

const PIN_SERVICE = "pin"

// ProtocolPinStat represents per-protocol aggregate pin counts.
type ProtocolPinStat struct {
	Protocol  string
	TotalPins uint64
}

type PinService interface {
	// Model registration methods
	// RegisterPinModel registers a protocol-specific pin data model for a protocol
	RegisterPinModel(protocol string, model data_models.PinDataModel)
	// GetPinModel retrieves the registered model for a protocol
	GetPinModel(protocol string) (data_models.PinDataModel, bool)
	// CreatePinModel creates a new instance of the registered model for a protocol
	CreatePinModel(protocol string) (data_models.PinDataModel, error)

	// AccountPins retrieves the list of pins (uploads) for the given user ID,
	// created after the specified timestamp.
	AccountPins(ctx context.Context, id uint, createdAfter uint64) ([]*models.Pin, error)

	// AllAccountPins retrieves all pins (uploads) for the given user ID.
	AllAccountPins(ctx context.Context, id uint) ([]*models.Pin, error)

	// DeletePinByHash deletes the pin associated with the given hash and user ID.
	DeletePinByHash(ctx context.Context, hash StorageHash, userId uint) error

	// GetPinByHash retrieves the pin associated with the given hash and user ID.
	GetPinByHash(ctx context.Context, hash StorageHash, userId uint) (*models.Pin, error)

	// PinByHash creates a new pin for the given hash and user ID if it doesn't exist.
	PinByHash(ctx context.Context, hash StorageHash, userId uint, protocolData any) error

	// PinByID creates a new pin for the given upload ID and user ID if it doesn't exist.
	PinByID(ctx context.Context, uploadId uint, userId uint, protocolData any) error

	// UploadPinnedGlobal checks if the upload with the given hash is pinned globally.
	UploadPinnedGlobal(ctx context.Context, hash StorageHash) (bool, error)

	// UploadPinnedByUser checks if the upload with the given hash is pinned by the specified user.
	UploadPinnedByUser(ctx context.Context, hash StorageHash, userId uint) (bool, error)

	// GetAllPinsByHash retrieves all pins for a given hash across all users.
	GetAllPinsByHash(ctx context.Context, hash StorageHash) ([]*models.Pin, error)

	// GetPinsByUploadID retrieves the list of pins for the given upload ID.
	GetPinsByUploadID(ctx context.Context, uploadID uint) ([]*models.Pin, error)

	// CreatePin creates a new pin or returns an existing one.
	CreatePin(ctx context.Context, pin *models.Pin, protocolData any) (*models.Pin, error)

	// UpdatePin updates a pin.
	UpdatePin(ctx context.Context, pin *models.Pin) error

	// GetPin retrieves a pin by ID.
	GetPin(ctx context.Context, id uint) (*models.Pin, error)

	// GetPinData retrieves the protocol-specific data for a pin
	GetPinData(ctx context.Context, pin *models.Pin) (interface{}, error)

	// DeletePin deletes a pin and its associated protocol data by ID
	DeletePin(ctx context.Context, id uint) error

	// QueryPin queries for a pin based on the provided query and filter
	QueryPin(ctx context.Context, query interface{}, filter PinFilter) (*models.Pin, error)

	// UpdatePinData updates the protocol-specific data for a pin
	UpdatePinData(ctx context.Context, pin *models.Pin, data interface{}) error

	// UpdateProtocolPin updates the protocol-specific data for a pin.
	UpdateProtocolPin(ctx context.Context, id uint, protocolData any) error

	// GetProtocolPin retrieves the protocol-specific data for a pin.
	GetProtocolPin(ctx context.Context, id uint) (any, error)

	//QueryProtocolData queries for protocol-specific data based on the provided query and filter.
	QueryProtocolPin(ctx context.Context, protocol string, query any, filter PinFilter) (any, error)

	// GetPinStats returns per-protocol aggregate pin counts.
	// Joins pins → uploads on upload_id, groups by uploads.protocol.
	// Soft-deleted uploads are excluded.
	GetPinStats(ctx context.Context) ([]ProtocolPinStat, error)

	Service
}

type PinFilter struct {
	UserID       uint
	UploadID     uint
	Hash         StorageHash
	CreatedAfter time.Time
	Limit        int
	Offset       int
	Protocol     string
	Status       string
}
