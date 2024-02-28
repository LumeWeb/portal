package config

import "errors"

var _ Defaults = (*EtcdConfig)(nil)

type EtcdConfig struct {
	Endpoints   []string `mapstructure:"endpoints"`
	DialTimeout int      `mapstructure:"dial_timeout"`
}

func (r *EtcdConfig) Validate() error {
	if len(r.Endpoints) == 0 {
		return errors.New("endpoints is required")
	}
	return nil
}

func (r *EtcdConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"dial_timeout": 5,
	}
}
