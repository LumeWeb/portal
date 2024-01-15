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

	defaults()

	err := viper.ReadInConfig()
	if err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			logger.Info("Config file not found, using default settings.")
			err := viper.SafeWriteConfig()
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

func defaults() {
	viper.SetDefault("core.post-upload-limit", 1024*1024*1000)
	viper.SetDefault("core.log.level", "info")
}
