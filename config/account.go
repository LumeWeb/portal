package config

var _ Defaults = (*AccountConfig)(nil)

type AccountConfig struct {
	DeletionGracePeriod uint `mapstructure:"deletion_grace_period"`
}

func (a AccountConfig) Defaults() map[string]any {
	return map[string]any{
		"deletion_grace_period": 24 * 2,
	}
}
