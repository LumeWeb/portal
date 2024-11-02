package config

import (
	"errors"
	"math/big"
)

var _ Validator = (*SiaConfig)(nil)
var _ Defaults = (*SiaConfig)(nil)

type SiaConfig struct {
	Key                string `config:"key"`
	URL                string `config:"url"`
	HostScoreAPIURL    string `config:"host_score_api_url"`
	MaxContractSCPrice string `config:"max_contract_sc_price"`
	MaxRPCSCPrice      string `config:"max_rpc_sc_price"`
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"key":                   "",
		"url":                   "",
		"host_score_api_url":    "https://api.hostscore.info",
		"max_rpc_sc_price":      1000,
		"max_contract_sc_price": 1,
	}
}

func (s SiaConfig) Validate() error {
	if s.Key == "" {
		return errors.New("core.storage.sia.key is required")
	}
	if s.URL == "" {
		return errors.New("core.storage.sia.url is required")
	}

	if err := validateStringNumber(s.MaxContractSCPrice, "core.storage.sia.max_contract_sc_price"); err != nil {
		return err
	}

	if err := validateStringNumber(s.MaxRPCSCPrice, "core.storage.sia.max_rpc_sc_price"); err != nil {
		return err
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
