package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*CronConfig)(nil)
	_ Defaults                           = (*CronConfig)(nil)
)

type CronConfig struct {
	Enabled  bool `config:"enabled"`
	MaxQueue uint `config:"queue_limit"`
}

func (c CronConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool().
			Default(true),
		"MaxQueue": z.Int().
			Default(50).
			GT(0, z.Message("queue limit must be greater than 0")),
	})
}

func (c CronConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":   true,
		"MaxQueue":  50,
	}
}
