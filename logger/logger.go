package logger

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var (
	logger *zap.Logger
)

func Get(viper *viper.Viper) *zap.Logger {
	if logger == nil {

		// Create a new atomic level
		atomicLevel := zap.NewAtomicLevel()

		// Set initial log level, for example, info level
		atomicLevel.SetLevel(mapLogLevel(viper.GetString("core.log.level")))

		// Create the logger with the atomic level
		logger = zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
			zapcore.Lock(os.Stdout),
			atomicLevel,
		))
	}

	return logger
}

func mapLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}
