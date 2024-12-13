package config

import (
	"errors"
	"math/big"
)

var _ Validator = (*SiaConfig)(nil)
var _ Defaults = (*SiaConfig)(nil)

type SiaConfig struct {
	Key     string `config:"key"`
	URL     string `config:"url"`
	Cluster bool   `config:"cluster"`
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"key":     "",
		"cluster": false,
		"url":     "",
	}
}

func (s SiaConfig) Validate() error {
	if s.Key == "" {
		return errors.New("core.storage.sia.key is required")
	}

	if s.Cluster && s.URL == "" {
		return errors.New("core.storage.sia.url is required")
	}

	return nil
}

func validateStringNumber(s string, name string) error {
	if s == "" {
		return errors.New(name + " is required")
	}

	rat, ok := new(big.Rat).SetString(s)
	if !ok {
		return errors.New("failed to parse " + name)
	}

	if rat.Cmp(new(big.Rat).SetUint64(0)) <= 0 {
		return errors.New(name + " must be greater than 0")
	}

	return nil
}
