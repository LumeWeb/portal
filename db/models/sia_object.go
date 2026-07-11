package models

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func init() {
	registerModel(&RenterObject{})
}

// RenterObjectStatus tracks the lifecycle state of a Sia object.
type RenterObjectStatus string

const (
	// RenterObjectStatusStaged: object data is in the staging backend, awaiting
	// packing into a slab by the background loop.
	RenterObjectStatusStaged RenterObjectStatus = "staged"
	// RenterObjectStatusPacking: the packing loop has picked up the object and is
	// uploading it. DeleteObject must not touch staging data in this state.
	RenterObjectStatusPacking RenterObjectStatus = "packing"
	// RenterObjectStatusUploaded: the object has been uploaded and pinned to Sia.
	// SiaObjectID and SealedData are populated; StagingKey is cleared.
	RenterObjectStatusUploaded RenterObjectStatus = "uploaded"
	// RenterObjectStatusDeleting: the object is being deleted. The packing loop
	// must not update it.
	RenterObjectStatusDeleting RenterObjectStatus = "deleting"
)

// RenterObject tracks both staged and uploaded Sia object state. State transitions
// are coordinated via the Status field using compare-and-swap (CAS) updates to
// prevent races between the packing loop and DeleteObject.
type RenterObject struct {
	gorm.Model
	Protocol    string `gorm:"uniqueIndex:idx_renter_object_key"`
	Bucket      string
	ObjectKey   string `gorm:"uniqueIndex:idx_renter_object_key"`
	Hash        []byte                  // raw multihash bytes (stored for reference, not indexed)
	Size        int64                    // object size in bytes
	SiaObjectID string                   // SDK object ID (types.Hash256 hex). Empty = staged.
	StagingKey  string                   // StagingBackend key. Empty = uploaded.
	SealedData  datatypes.JSON           // sealed object metadata for SDK reconstruction
	Status      RenterObjectStatus      // lifecycle state: staged, packing, uploaded, deleting
}

func (RenterObject) TableName() string { return "renter_objects" }
