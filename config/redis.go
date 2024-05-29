package config

import (
	"errors"
	"github.com/redis/go-redis/v9"
)

var _ Validator = (*RedisConfig)(nil)
var _ Defaults = (*RedisConfig)(nil)

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	client   *redis.Client
}

func (r *RedisConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"address": "localhost:6379",
		"db":      0,
	}
}

func (r *RedisConfig) Validate() error {
	if r.Address == "" {
		return errors.New("address is required")
	}
	return nil
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
