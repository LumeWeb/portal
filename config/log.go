package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*LogConfig)(nil)
	_ Defaults                           = (*LogConfig)(nil)
)

type LogConfig struct {
	Level string `config:"level"`
}

func (l LogConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Level": z.String().
			Default("info").
			OneOf([]string{"debug", "info", "warn", "error", "fatal"}, z.Message("log level must be one of: debug, info, warn, error, fatal")),
	})
}

func (l LogConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"Level": "info",
	}
}
