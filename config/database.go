package config

import (
	"reflect"

	"github.com/mitchellh/mapstructure"
)

type DatabaseConfig struct {
	Charset  string       `mapstructure:"charset"`
	Host     string       `mapstructure:"host"`
	Name     string       `mapstructure:"name"`
	Password string       `mapstructure:"password"`
	Port     int          `mapstructure:"port"`
	Username string       `mapstructure:"username"`
	Cache    *CacheConfig `mapstructure:"cache"`
}

type CacheConfig struct {
	Mode    string      `mapstructure:"mode"`
	Options interface{} `mapstructure:"options"`
}

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type MemoryConfig struct {
}

func cacheConfigHook() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		// This hook is designed to operate on the options field within the CacheConfig
		if f.Kind() != reflect.Map || t != reflect.TypeOf(&CacheConfig{}) {
			return data, nil
		}

		var cacheConfig CacheConfig
		if err := mapstructure.Decode(data, &cacheConfig); err != nil {
			return nil, err
		}

		// Assuming the input data map includes "mode" and "options"
		switch cacheConfig.Mode {
		case "redis":
			var redisOptions RedisConfig
			if opts, ok := cacheConfig.Options.(map[string]interface{}); ok && opts != nil {
				if err := mapstructure.Decode(opts, &redisOptions); err != nil {
					return nil, err
				}
				cacheConfig.Options = redisOptions
			}
		case "memory":
			// For "memory", you might simply use an empty MemoryConfig,
			// or decode options similarly if there are any specific to memory caching.
			cacheConfig.Options = MemoryConfig{}
		case "false":
			// If "false", ensure no options are set, or set to a nil or similar neutral value.
			cacheConfig.Options = nil
		default:
			cacheConfig.Options = nil
		}

		return cacheConfig, nil
	}
}
