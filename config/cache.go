package config

import (
	"github.com/go-viper/mapstructure/v2"
	"reflect"
)

var _ Defaults = (*CacheConfig)(nil)

type CacheMode string

const (
	CacheModeMemory CacheMode = "memory"
	CacheModeRedis  CacheMode = "redis"
)

type CacheConfig struct {
	Mode    CacheMode   `mapstructure:"mode"`
	Options interface{} `mapstructure:"options"`
}

func (c CacheConfig) Defaults() map[string]any {
	return map[string]any{
		"mode":    "memory",
		"options": MemoryConfig{},
	}
}

type MemoryConfig struct {
}

func cacheConfigHook(cm *ManagerDefault) mapstructure.DecodeHookFuncType {
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
		case CacheModeRedis:
			if cm.Config().Core.Clustered.Enabled {
				cm.Config().Core.DB.Cache.Options = cm.Config().Core.Clustered.Redis
				return cacheConfig, nil
			}

			var redisOptions RedisConfig
			if opts, ok := cacheConfig.Options.(map[string]interface{}); ok && opts != nil {
				if err := mapstructure.Decode(opts, &redisOptions); err != nil {
					return nil, err
				}
				cacheConfig.Options = redisOptions
			}
		case CacheModeMemory:
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
