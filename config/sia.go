package config

import (
	"errors"
	"math/big"
)

var _ Validator = (*SiaConfig)(nil)
var _ Defaults = (*SiaConfig)(nil)

type SiaConfig struct {
	Key                string  `mapstructure:"key"`
	URL                string  `mapstructure:"url"`
	PriceHistoryDays   uint64  `mapstructure:"price_history_days"`
	MaxUploadPrice     float64 `mapstructure:"max_upload_price"`
	MaxDownloadPrice   float64 `mapstructure:"max_download_price"`
	MaxStoragePrice    float64 `mapstructure:"max_storage_price"`
	MaxContractSCPrice float64 `mapstructure:"max_contract_sc_price"`
	MaxRPCSCPrice      string  `mapstructure:"max_rpc_sc_price"`
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
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

	if s.MaxUploadPrice <= 0 {
		return errors.New("core.storage.sia.max_upload_price must be greater than 0")
	}

	if s.MaxDownloadPrice <= 0 {
		return errors.New("core.storage.sia.max_download_price must be greater than 0")
	}

	if s.MaxStoragePrice <= 0 {
		return errors.New("core.storage.sia.max_storage_price must be greater than 0")
	}

	err := errors.New("failed to parse core.storage.sia.max_rpc_sc_price ")

	rat, ok := new(big.Rat).SetString(s.MaxRPCSCPrice)
	if !ok {
		return err
	}

	if rat.Cmp(new(big.Rat).SetUint64(0)) <= 0 {
		return err
	}

	return nil
}
