package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*CacheConfig)(nil)
	_ Defaults                           = (*CacheConfig)(nil)
)

type CacheMode string

const (
	CacheModeMemory CacheMode = "memory"
	CacheModeRedis  CacheMode = "redis"
	CacheModeNone   CacheMode = "none"
)

type CacheConfig struct {
	Mode    CacheMode   `config:"mode"`
	Options interface{} `config:"options"`
}

func (c CacheConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Mode": z.String().
			OneOf([]string{string(CacheModeMemory), string(CacheModeRedis), string(CacheModeNone)}, z.Message("must be one of: memory, redis, none")),
	})
}

func (c CacheConfig) Defaults() map[string]any {
	return map[string]any{
		"Mode":    CacheModeMemory,
		"Options": MemoryConfig{},
	}
}

type MemoryConfig struct {
}
