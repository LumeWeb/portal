package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*OAuthConfig)(nil)
	_ Defaults                           = (*OAuthConfig)(nil)
)

// OAuthConfig configures the OAuth 2.1 authorization-server (IdP) provider.
// The provider is disabled by default; the API layer enables it. TTL fields
// are time.ParseDuration-compatible strings.
type OAuthConfig struct {
	Enabled    bool   `config:"enabled"`
	Issuer     string `config:"issuer"`      // Override auto-detected issuer
	TokenTTL   string `config:"token_ttl"`   // Access token lifetime; default 24h
	RefreshTTL string `config:"refresh_ttl"` // Refresh token lifetime; default 720h (30d)
	CodeTTL    string `config:"code_ttl"`    // Authorization code lifetime; default 10m
}

func (o OAuthConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
		"Issuer":  z.String(),
		"TokenTTL": z.String().
			Optional(),
		"RefreshTTL": z.String().
			Optional(),
		"CodeTTL": z.String().
			Optional(),
	})
}

func (o OAuthConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":    false,
		"Issuer":     "",
		"TokenTTL":   "24h",
		"RefreshTTL": "720h",
		"CodeTTL":    "10m",
	}
}
