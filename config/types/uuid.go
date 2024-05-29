package types

import (
	"errors"
	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type UUID uuid.UUID

var _ yaml.Marshaler = (*UUID)(nil)
var _ mapstructure.Unmarshaler = (*UUID)(nil)

func NewUUID() UUID {
	return UUID(uuid.New())
}

func (u UUID) MarshalYAML() (interface{}, error) {
	return uuid.UUID(u).String(), nil
}

func (u *UUID) UnmarshalYAML(value *yaml.Node) error {
	id, err := uuid.Parse(value.Value)
	if err != nil {
		return err
	}

	*u = UUID(id)
	return nil

}

func (u *UUID) DecodeMapstructure(value interface{}) error {
	switch v := value.(type) {
	case string:
		id, err := uuid.Parse(v)
		if err != nil {
			return err
		}
		*u = UUID(id)
		return nil
	default:
		return errors.New("unsupported type for UUID")
	}
}

func (u UUID) Bytes() []byte {
	var b []byte
	c := uuid.UUID(u)
	b = append(b, c[:]...)
	return b
}

func ParseUUID(s string) (UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return UUID{}, err
	}

	return UUID(id), nil
}

func (u UUID) String() string {
	return uuid.UUID(u).String()
}
