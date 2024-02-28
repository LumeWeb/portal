package config

import "errors"

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
