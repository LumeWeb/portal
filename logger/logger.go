package logger

import (
	"os"

	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(cm *config.Manager) (*zap.Logger, *zap.AtomicLevel) {

	// Create a new atomic level
	atomicLevel := zap.NewAtomicLevel()

	// Set initial log level, for example, info level
	atomicLevel.SetLevel(mapLogLevel(cm.Config().Core.Log.Level))

	// Create the logger with the atomic level
	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.Lock(os.Stdout),
		atomicLevel,
	))

	return logger, &atomicLevel
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
