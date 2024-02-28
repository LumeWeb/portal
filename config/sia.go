package config

import "errors"

var _ Validator = (*SiaConfig)(nil)

type SiaConfig struct {
	Key string `mapstructure:"key"`
	URL string `mapstructure:"url"`
}

func (s SiaConfig) Validate() error {
	if s.Key == "" {
		return errors.New("core.storage.sia.key is required")
	}
	if s.URL == "" {
		return errors.New("core.storage.sia.url is required")
	}
	return nil
}
