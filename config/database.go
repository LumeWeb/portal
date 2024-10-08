package config

import (
	"errors"
)

var _ Defaults = (*DatabaseConfig)(nil)
var _ Validator = (*DatabaseConfig)(nil)

type DatabaseConfig struct {
	Type     string       `config:"type"`
	File     string       `config:"file"`
	Charset  string       `config:"charset"`
	Host     string       `config:"host"`
	Name     string       `config:"name"`
	Password string       `config:"password"`
	Port     int          `config:"port"`
	Username string       `config:"username"`
	Cache    *CacheConfig `config:"cache"`
}

func (d DatabaseConfig) CacheEnabled() bool {
	return d.Cache != nil && d.Cache.Mode != CacheModeNone
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

func (d DatabaseConfig) Defaults() map[string]any {
	def := map[string]any{}

	if d.Type == "sqlite" || d.Type == "" {
		def["file"] = "portal.db"
	}

	if d.Type == "mysql" {
		def["host"] = "localhost"
		def["port"] = 3306
		def["charset"] = "utf8mb4"
		def["name"] = "portal"
	}

	return def
}
