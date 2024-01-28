package main

import (
	"crypto/ed25519"
	"github.com/spf13/viper"
	"go.sia.tech/core/wallet"
	"go.uber.org/zap"
)

func initCheckRequiredConfig(logger *zap.Logger, config *viper.Viper) error {
	required := []string{
		"core.domain",
		"core.port",
		"core.sia.url",
		"core.sia.key",
		"core.db.username",
		"core.db.password",
		"core.db.host",
		"core.db.name",
		"core.storage.s3.bufferBucket",
		"core.storage.s3.endpoint",
		"core.storage.s3.region",
		"core.storage.s3.accessKey",
		"core.storage.s3.secretKey",
	}

	for _, key := range required {
		if !config.IsSet(key) {
			logger.Fatal(key + " is required")
		}
	}

	return nil
}

func NewIdentity(config *viper.Viper, logger *zap.Logger) (ed25519.PrivateKey, error) {
	var seed [32]byte
	identitySeed := config.GetString("core.identity")

	if identitySeed == "" {
		logger.Info("Generating new identity seed")
		identitySeed = wallet.NewSeedPhrase()
		config.Set("core.identity", identitySeed)
		err := config.WriteConfig()
		if err != nil {
			return nil, err
		}
	}
	err := wallet.SeedFromPhrase(&seed, identitySeed)
	if err != nil {
		return nil, err
	}

	return ed25519.PrivateKey(wallet.KeyFromSeed(&seed, 0)), nil
}
