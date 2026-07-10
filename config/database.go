package config

import (
	"time"

	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*DatabaseConfig)(nil)
	_ Defaults                           = (*DatabaseConfig)(nil)
)

type DatabaseConfig struct {
	Type            string        `config:"type"`
	File            string        `config:"file"`
	Charset         string        `config:"charset"`
	Host            string        `config:"host"`
	Name            string        `config:"name"`
	Password        string        `config:"password"`
	Port            int           `config:"port"`
	Username        string        `config:"username"`
	Cache           *CacheConfig  `config:"cache"`
	TLSEnabled      bool          `config:"tls_enabled"`
	TLSSkipVerify   bool          `config:"tls_skip_verify"`
	MetricsPort     uint          `config:"metrics_port"`
	MaxOpenConns    int           `config:"max_open_conns"`
	MaxIdleConns    int           `config:"max_idle_conns"`
	ConnMaxLifetime time.Duration `config:"conn_max_lifetime"`
}

func (d DatabaseConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Type": z.String().
			OneOf([]string{"sqlite", "mysql"}),
		"File":          z.String(),
		"Host":          z.String(),
		"Port":          z.Int(),
		"Username":      z.String(),
		"Password":      z.String(),
		"Name":          z.String(),
		"Charset":       z.String(),
		"TLSEnabled":    z.Bool(),
		"TLSSkipVerify": z.Bool(),
		"MetricsPort": z.Uint().
			GT(0, z.Message("metrics_port must be greater than 0")),
		"MaxOpenConns": z.Int().GT(0, z.Message("max_open_conns must be greater than 0")),
		"MaxIdleConns": z.Int().GT(0, z.Message("max_idle_conns must be greater than 0")),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		d, ok := data.(*DatabaseConfig)
		if !ok {
			return true
		}

		valid := true

		if d.Type == "sqlite" {
			if d.File == "" {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"File"}).SetMessage("core.db.file is required for sqlite"))
				valid = false
			}
		} else if d.Type == "mysql" {
			if d.Host == "" {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"Host"}).SetMessage("core.db.host is required for mysql"))
				valid = false
			}
			if d.Port <= 0 {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"Port"}).SetMessage("core.db.port must be greater than 0"))
				valid = false
			}
			if d.Username == "" {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"Username"}).SetMessage("core.db.username is required for mysql"))
				valid = false
			}
			if d.Password == "" {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"Password"}).SetMessage("core.db.password is required for mysql"))
				valid = false
			}
			if d.Name == "" {
				ctx.AddIssue(ctx.Issue().SetPath([]string{"Name"}).SetMessage("core.db.name is required for mysql"))
				valid = false
			}
		}
		return valid
	})
}

func (d DatabaseConfig) CacheEnabled() bool {
	return d.Cache != nil && d.Cache.Mode != CacheModeNone
}

func (d DatabaseConfig) Defaults() map[string]any {
	def := map[string]any{
		"Type":            "sqlite",
		"File":            "portal.db",
		"Charset":         "utf8mb4",
		"MetricsPort":     uint(9091),
		"MaxOpenConns":    25,
		"MaxIdleConns":    10,
		"ConnMaxLifetime": "5m",
	}

	if d.Type == "mysql" {
		def["Host"] = "localhost"
		def["Port"] = 3306
		def["Name"] = "portal"
	}

	return def
}
