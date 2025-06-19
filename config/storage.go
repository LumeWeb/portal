package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*StorageConfig)(nil)
	_ Defaults                           = (*StorageConfig)(nil)
)

type StorageConfig struct {
	S3  S3Config  `config:"s3"`
	Sia SiaConfig `config:"sia"`
	Tus TusConfig `config:"tus"`
}

func (s StorageConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{})
}

func (s StorageConfig) Defaults() map[string]any {
	return map[string]any{
		"S3":  S3Config{},
		"Sia": SiaConfig{},
		"Tus": TusConfig{},
	}
}
