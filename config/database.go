package config

import (
	"errors"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

var _ Defaults = (*DatabaseConfig)(nil)
var _ Validator = (*DatabaseConfig)(nil)

type DatabaseConfig struct {
	Type     string       `mapstructure:"type"`
	File     string       `mapstructure:"file"`
	Charset  string       `mapstructure:"charset"`
	Host     string       `mapstructure:"host"`
	Name     string       `mapstructure:"name"`
	Password string       `mapstructure:"password"`
	Port     int          `mapstructure:"port"`
	Username string       `mapstructure:"username"`
	Cache    *CacheConfig `mapstructure:"cache"`
}

func (d DatabaseConfig) Validate() error {
	if d.Type == "sqlite" {
		if d.File == "" {
			return errors.New("core.db.file is required")
		}
	}

	if d.Type == "mysql" {
		if d.Host == "" {
			return errors.New("core.db.host is required")
		}
		if d.Port == 0 {
			return errors.New("core.db.port is required")
		}
		if d.Username == "" {
			return errors.New("core.db.username is required")
		}
		if d.Password == "" {
			return errors.New("core.db.password is required")
		}
		if d.Name == "" {
			return errors.New("core.db.name is required")
		}
	}

	return nil
}

func (d DatabaseConfig) Defaults() map[string]interface{} {
	def := map[string]interface{}{
		"type":    "sqlite",
		"host":    "localhost",
		"charset": "utf8mb4",
		"port":    3306,
		"name":    "portal",
	}

	if d.Type == "sqlite" || d.Type == "" {
		def["file"] = "portal.db"
	}

	return def
}

type CacheConfig struct {
	Mode    string      `mapstructure:"mode"`
	Options interface{} `mapstructure:"options"`
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
		case "redis":
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
