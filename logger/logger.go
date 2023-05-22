package logger

import (
	"go.uber.org/zap"
	"log"
)

var logger *zap.Logger

func Init() {
	newLogger, err := zap.NewProduction()

	if err != nil {
		log.Fatal(err)
	}

	logger = newLogger
}

func Get() *zap.Logger {
	return logger
}
