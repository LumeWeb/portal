package core

import (
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

type Logger struct {
	*zap.Logger
	level *zap.AtomicLevel
	cm    config.Manager
}

func NewLogger(cm config.Manager, existingLogger ...any) *Logger {
	// Create a new atomic level
	atomicLevel := zap.NewAtomicLevel()

	if cm != nil && cm.Config() != nil {
		// Set initial log level, for example, info level
		atomicLevel.SetLevel(mapLogLevel(cm.Config().Core.Log.Level))
	} else {
		atomicLevel.SetLevel(mapLogLevel("debug"))
	}

	// Create the logger with the atomic level
	zapLogger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.Lock(os.Stdout),
		atomicLevel,
	), zap.AddCaller())

	logger := &Logger{
		Logger: zapLogger,
		level:  &atomicLevel,
		cm:     cm,
	}

	// If an existing logger is provided, use it instead
	if len(existingLogger) > 0 {
		switch v := existingLogger[0].(type) {
		case *Logger:
			logger.Logger = v.Logger
			logger.level = v.Level()
			logger.cm = cm
		case *zap.Logger:
			logger.Logger = v
			atomicLevel.SetLevel(v.Level())
		}
	}

	// Only set the logger on the config manager if it's not nil
	if cm != nil {
		cm.SetLogger(zapLogger)
	}

	return logger
}

func (l *Logger) SetLevelFromConfig() {
	if l.cm != nil && l.cm.Config() != nil {
		l.level.SetLevel(mapLogLevel(l.cm.Config().Core.Log.Level))
	}
}

func (l *Logger) Level() *zap.AtomicLevel {
	return l.level
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
