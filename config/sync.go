package config

var _ Defaults = (*SyncConfig)(nil)

type SyncConfig struct {
	Enabled bool `config:"enabled"`
}

func (s SyncConfig) Defaults() map[string]any {
	return map[string]any{
		"enabled": false,
	}
}
