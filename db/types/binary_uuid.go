package types

import (
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Credits to https://github.com/dipeshdulal/binary-uuid-gorm

// BinaryUUID -> binary uuid wrapper over uuid.UUID
type BinaryUUID uuid.UUID

// ParseUUID -> parses string uuid to binary uuid
func ParseUUID(id string) BinaryUUID {
	return BinaryUUID(uuid.MustParse(id))
}

func (b BinaryUUID) String() string {
	return uuid.UUID(b).String()
}

// MarshalJSON -> convert to json string
func (b BinaryUUID) MarshalJSON() ([]byte, error) {
	s := uuid.UUID(b)
	str := "\"" + s.String() + "\""
	return []byte(str), nil
}

// UnmarshalJSON -> convert from json string
func (b *BinaryUUID) UnmarshalJSON(by []byte) error {
	s, err := uuid.ParseBytes(by)
	*b = BinaryUUID(s)
	return err
}

// GormDataType -> sql data type for gorm
func (BinaryUUID) GormDataType() string {
	return "binary(16)"
}

// Scan -> scan value into BinaryUUID
func (b *BinaryUUID) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	data, err := uuid.FromBytes(bytes)
	*b = BinaryUUID(data)
	return err
}

// Value -> return BinaryUUID to []bytes binary(16)
func (b BinaryUUID) Value() (driver.Value, error) {
	return uuid.UUID(b).MarshalBinary()
}
