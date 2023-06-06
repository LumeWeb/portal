package logger

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"log"
)

var logger *zap.Logger

func Init() {
	var newLogger *zap.Logger
	var err error

	if viper.GetBool("debug") {
		newLogger, err = zap.NewDevelopment()
	} else {
		newLogger, err = zap.NewProduction()
	}

	if err != nil {
		log.Fatal(err)
	}

	logger = newLogger
}

func Get() *zap.Logger {
	return logger
}
