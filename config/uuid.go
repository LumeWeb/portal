package config

import (
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type UUID uuid.UUID

var _ yaml.Marshaler = (*UUID)(nil)

func NewUUID() UUID {
	return UUID(uuid.New())
}

func (u UUID) MarshalYAML() (interface{}, error) {
	return uuid.UUID(u).String(), nil
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
