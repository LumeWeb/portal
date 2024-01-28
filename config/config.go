package config

import (
	"errors"
	_logger "git.lumeweb.com/LumeWeb/portal/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	ConfigFilePaths = []string{
		"/etc/lumeweb/portal/",
		"$HOME/.lumeweb/portal/",
		".",
	}
)

func NewConfig(logger *zap.Logger) (*viper.Viper, error) {
	if logger == nil {
		logger = _logger.NewFallbackLogger()
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	for _, path := range ConfigFilePaths {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("LUME_WEB_PORTAL")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			logger.Info("Config file not found, using default settings.")
			err := viper.SafeWriteConfig()
			if err != nil {
				return nil, err
			}
			err = writeDefaults()
			if err != nil {
				return nil, err
			}

			return viper.GetViper(), nil
		}
		return nil, err
	}

	err = writeDefaults()
	if err != nil {
		return nil, err
	}

	return viper.GetViper(), nil
}

func writeDefaults() error {
	defaults := map[string]interface{}{
		"core.post-upload-limit":                  1024 * 1024 * 1000,
		"core.log.level":                          "info",
		"core.db.charset":                         "utf8mb4",
		"core.db.port":                            3306,
		"core.db.name":                            "portal",
		"protocol.s5.p2p.maxOutgoingPeerFailures": 10,
		"protocol.s5.p2p.network":                 "",
	}

	changes := false

	for key, value := range defaults {
		if writeDefault(key, value) {
			changes = true
		}
	}

	if changes {
		err := viper.WriteConfig()
		if err != nil {
			return err
		}
	}

	return nil
}

func writeDefault(key string, value interface{}) bool {
	if !viper.IsSet(key) {
		viper.SetDefault(key, value)
		return true
	}

	return false
}
