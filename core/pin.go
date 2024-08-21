package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"time"
)

const PIN_SERVICE = "pin"

type PinService interface {
	// AccountPins retrieves the list of pins (uploads) for the given user ID,
	// created after the specified timestamp.
	AccountPins(id uint, createdAfter uint64) ([]*models.Pin, error)

	// DeletePinByHash deletes the pin associated with the given hash and user ID.
	DeletePinByHash(hash StorageHash, userId uint) error

	// PinByHash creates a new pin for the given hash and user ID if it doesn't exist.
	PinByHash(hash StorageHash, userId uint, protocolData any) error

	// PinByID creates a new pin for the given upload ID and user ID if it doesn't exist.
	PinByID(uploadId uint, userId uint, protocolData any) error

	// UploadPinnedGlobal checks if the upload with the given hash is pinned globally.
	UploadPinnedGlobal(hash StorageHash) (bool, error)

	// UploadPinnedByUser checks if the upload with the given hash is pinned by the specified user.
	UploadPinnedByUser(hash StorageHash, userId uint) (bool, error)

	// GetPinsByUploadID retrieves the list of pins for the given upload ID.
	GetPinsByUploadID(ctx context.Context, uploadID uint) ([]*models.Pin, error)

	// CreatePin creates a new pin or returns an existing one.
	CreatePin(ctx context.Context, pin *models.Pin, protocolData any) (*models.Pin, error)

	// UpdatePin updates a pin.
	UpdatePin(ctx context.Context, pin *models.Pin) error

	// GetPin retrieves a pin by ID.
	GetPin(ctx context.Context, id uint) (*models.Pin, error)

	// DeletePin deletes a pin by ID.
	DeletePin(ctx context.Context, id uint) error

	// QueryPin queries for a pin based on the provided query and filter.
	QueryPin(ctx context.Context, query interface{}, filter PinFilter) (*models.Pin, error)

	// UpdateProtocolPin updates the protocol-specific data for a pin.
	UpdateProtocolPin(ctx context.Context, id uint, protocolData any) error

	// GetProtocolPin retrieves the protocol-specific data for a pin.
	GetProtocolPin(ctx context.Context, id uint) (any, error)

	//QueryProtocolData queries for protocol-specific data based on the provided query and filter.
	QueryProtocolPin(ctx context.Context, protocol string, query any, filter PinFilter) (any, error)

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
