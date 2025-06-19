package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*AccountConfig)(nil)
	_ Defaults                           = (*AccountConfig)(nil)
)

type AccountConfig struct {
	DeletionGracePeriod uint `config:"deletion_grace_period"`
}

func (a AccountConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"DeletionGracePeriod": z.Int().
			GT(0, z.Message("deletion grace period must be greater than 0")),
	})
}

func (a AccountConfig) Defaults() map[string]any {
	return map[string]any{
		"DeletionGracePeriod": 48, // 24 * 2 hours
	}
}
