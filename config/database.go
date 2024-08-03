package config

import (
	"errors"
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
