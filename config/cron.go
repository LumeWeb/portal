package config

var _ Defaults = (*CronConfig)(nil)

type CronConfig struct {
	Enabled  bool `mapstructure:"enabled"`
	MaxQueue uint `mapstructure:"queue_limit"`
}

func (c CronConfig) Defaults() map[string]any {
	return map[string]any{
		"enabled":     true,
		"queue_limit": 50,
	}
}
