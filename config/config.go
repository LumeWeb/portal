package config

import (
	"errors"
	"fmt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
)

var (
	ConfigFilePaths = []string{
		"/etc/lumeweb/portal/",
		"$HOME/.lumeweb/portal/",
		".",
	}
)

func Init() {
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	for _, path := range ConfigFilePaths {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("LUME_WEB_PORTAL")
	viper.AutomaticEnv()

	pflag.String("database.type", "sqlite", "Database type")
	pflag.String("database.host", "localhost", "Database host")
	pflag.Int("database.port", 3306, "Database port")
	pflag.String("database.user", "root", "Database user")
	pflag.String("database.password", "", "Database password")
	pflag.String("database.name", "lumeweb_portal", "Database name")
	pflag.String("database.path", "./db.sqlite", "Database path for SQLite")
	pflag.String("renterd-api-password", ".", "admin password for renterd")
	pflag.Bool("debug", false, "enable debug mode")
	pflag.Parse()

	err := viper.BindPFlags(pflag.CommandLine)

	if err != nil {
		log.Fatalf("Fatal error arguments: %s \n", err)
		return
	}

	err = viper.ReadInConfig()
	if err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			// Config file not found, this is not an error.
			fmt.Println("Config file not found, using default settings.")
		} else {
			// Other error, panic.
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}
	}

}
