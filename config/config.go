package config

import (
	"errors"
	"fmt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
)

func Init() {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath("/etc/lumeweb/portal/")
	viper.AddConfigPath("$HOME/.lumeweb/portal/")
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("LUME_WEB_PORTAL")

	pflag.String("database.type", "sqlite", "Database type")
	pflag.String("database.host", "localhost", "Database host")
	pflag.Int("database.port", 3306, "Database port")
	pflag.String("database.user", "root", "Database user")
	pflag.String("database.password", "", "Database password")
	pflag.String("database.name", "lumeweb_portal", "Database name")
	pflag.String("database.path", "./db.sqlite", "Database path for SQLite")
	pflag.Parse()

	err := viper.BindPFlags(pflag.CommandLine)

	if err != nil {
		log.Fatalf("Fatal error arguments: %s \n", err)
		return
	}

	err = viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

}
