package main

import "github.com/spf13/viper"

var (
	ConfigFilePaths = []string{
		"/etc/lumeweb/portal/",
		"$HOME/.lumeweb/portal/",
		".",
	}
)

func Init() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	for _, path := range ConfigFilePaths {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("LUME_WEB_PORTAL")
	viper.AutomaticEnv()
}
