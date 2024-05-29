package core

import "github.com/LumeWeb/portal/db/models"

type PinService interface {
	// AccountPins retrieves the list of pins (uploads) for the given user ID,
	// created after the specified timestamp.
	AccountPins(id uint, createdAfter uint64) ([]models.Pin, error)

	// DeletePinByHash deletes the pin associated with the given hash and user ID.
	DeletePinByHash(hash []byte, userId uint) error

	// PinByHash creates a new pin for the given hash and user ID if it doesn't exist.
	PinByHash(hash []byte, userId uint) error

	// PinByID creates a new pin for the given upload ID and user ID if it doesn't exist.
	PinByID(uploadId uint, userId uint) error

	// UploadPinnedGlobal checks if the upload with the given hash is pinned globally.
	UploadPinnedGlobal(hash []byte) (bool, error)

	// UploadPinnedByUser checks if the upload with the given hash is pinned by the specified user.
	UploadPinnedByUser(hash []byte, userId uint) (bool, error)
}
