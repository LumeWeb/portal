package config

var _ Defaults = (*SyncConfig)(nil)

type SyncConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

func (s SyncConfig) Defaults() map[string]any {
	return map[string]any{
		"enabled": false,
	}
}
