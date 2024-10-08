package config

var _ Defaults = (*LogConfig)(nil)

type LogConfig struct {
	Level string `config:"level"`
}

func (l LogConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"level": "info",
	}
}
