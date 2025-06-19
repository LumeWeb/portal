package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*SyncConfig)(nil)
	_ Defaults                           = (*SyncConfig)(nil)
)

type SyncConfig struct {
	Enabled bool `config:"enabled"`
}

func (s SyncConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool().
			Default(false),
	})
}

func (s SyncConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled": false,
	}
}
