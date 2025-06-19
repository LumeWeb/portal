package config

import (
	z "github.com/Oudwins/zog"
	"github.com/redis/go-redis/v9"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*RedisConfig)(nil)
	_ Defaults                           = (*RedisConfig)(nil)
)

type RedisConfig struct {
	Address  string `config:"address"`
	Password string `config:"password"`
	DB       int    `config:"db"`
	client   *redis.Client
}

func (r RedisConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Address": z.String().
			Required(z.Message("address is required")),
		"Password": z.String(),
		"DB": z.Int().
			Default(0),
	})
}

func (r *RedisConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"Address": "localhost:6379",
		"DB":      0,
	}
}

func (r *RedisConfig) Client() (*redis.Client, error) {
	if r.client == nil {
		r.client = redis.NewClient(&redis.Options{
			Addr:     r.Address,
			Password: r.Password,
		})
	}

	return r.client, nil
}
