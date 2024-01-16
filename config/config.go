package config

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/spf13/viper"
)

var (
	ConfigFilePaths = []string{
		"/etc/lumeweb/portal/",
		"$HOME/.lumeweb/portal/",
		".",
	}
)

func Init(p interfaces.Portal) error {
	logger := p.Logger()
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
