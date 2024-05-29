package config

import (
	"errors"
	"math/big"
)

var _ Validator = (*SiaConfig)(nil)
var _ Defaults = (*SiaConfig)(nil)

type SiaConfig struct {
	Key                string `mapstructure:"key"`
	URL                string `mapstructure:"url"`
	PriceHistoryDays   uint64 `mapstructure:"price_history_days"`
	MaxUploadPrice     string `mapstructure:"max_upload_price"`
	MaxDownloadPrice   string `mapstructure:"max_download_price"`
	MaxStoragePrice    string `mapstructure:"max_storage_price"`
	MaxContractSCPrice string `mapstructure:"max_contract_sc_price"`
	MaxRPCSCPrice      string `mapstructure:"max_rpc_sc_price"`
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"key":                   "",
		"url":                   "",
		"max_upload_price":      0,
		"max_download_price":    0,
		"max_storage_price":     0,
		"max_rpc_sc_price":      0.1,
		"max_contract_sc_price": 1,
		"price_history_days":    90,
	}
}

func (s SiaConfig) Validate() error {
	if s.Key == "" {
		return errors.New("core.storage.sia.key is required")
	}
	if s.URL == "" {
		return errors.New("core.storage.sia.url is required")
	}

	if err := validateStringNumber(s.MaxUploadPrice, "core.storage.sia.max_upload_price"); err != nil {
		return err
	}

	if err := validateStringNumber(s.MaxDownloadPrice, "core.storage.sia.max_download_price"); err != nil {
		return err
	}

	if err := validateStringNumber(s.MaxStoragePrice, "core.storage.sia.max_storage_price"); err != nil {
		return err
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
