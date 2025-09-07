package types

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

var EmptyUUID = FromUUID(uuid.Nil)

// BinaryUUID is a simple wrapper around datatypes.BinUUID with JSON support
type BinaryUUID struct {
	datatypes.BinUUID
}

// ParseUUID parses string uuid to binary uuid
func ParseUUID(id string) BinaryUUID {
	if id == "" {
		return BinaryUUID{}
	}
	return BinaryUUID{BinUUID: datatypes.BinUUIDFromString(id)}
}

// NewBinUUID generates a new random (v4) UUID and returns it as a BinaryUUID
func NewBinUUID() BinaryUUID {
	return BinaryUUID{BinUUID: datatypes.NewBinUUIDv4()}
}

// FromUUID converts a raw uuid.UUID to BinaryUUID
func FromUUID(id uuid.UUID) BinaryUUID {
	return BinaryUUID{BinUUID: datatypes.BinUUID(id)}
}

// MarshalJSON converts to JSON string
func (b BinaryUUID) ToUUID() uuid.UUID {
	return uuid.UUID(b.BinUUID)
}

func (b BinaryUUID) ToUUIDRaw() []byte {
	return b.BinUUID[:]
}

func (b BinaryUUID) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// UnmarshalJSON converts from JSON string
func (b *BinaryUUID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return err
	}
	b.BinUUID = datatypes.BinUUID(id)
	return nil
}

func (u BinaryUUID) Equals(other BinaryUUID) bool {
	return u.BinUUID.Equals(other.BinUUID)
}

func (u BinaryUUID) Empty() bool {
	return u.Equals(EmptyUUID)
}
