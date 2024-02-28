package config

import "errors"

var _ Validator = (*RedisConfig)(nil)
var _ Defaults = (*RedisConfig)(nil)

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
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
