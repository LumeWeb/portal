package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*TusConfig)(nil)
	_ Defaults                           = (*TusConfig)(nil)
)

type TusConfig struct {
	LockerMode string `config:"locker_mode"`
}

func (t TusConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"LockerMode": z.String().
			Default("db").
			OneOf([]string{"db", "redis"}, z.Message("tus_locker_mode must be one of: db, redis")),
	})
}

func (t TusConfig) Defaults() map[string]any {
	return map[string]any{
		"LockerMode": "db",
	}
}
