package config

import (
	"errors"
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

func Init() error {
	logger, _ := zap.NewDevelopment()
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
				return err
			}
			return writeDefaults()
		}
		return err
	}

	return writeDefaults()
}

func writeDefaults() error {
	defaults := map[string]interface{}{
		"core.post-upload-limit":                  1024 * 1024 * 1000,
		"core.log.level":                          "info",
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
