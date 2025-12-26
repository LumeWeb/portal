package core

import (
	"os"
	"sync/atomic"

	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/log/global"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type levelFilterCore struct {
	zapcore.Core
	minLevel atomic.Int32
}

func (c *levelFilterCore) Enabled(lvl zapcore.Level) bool {
	return lvl >= zapcore.Level(c.minLevel.Load()) && c.Core.Enabled(lvl)
}

func (c *levelFilterCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *levelFilterCore) SetMinLevel(lvl zapcore.Level) {
	c.minLevel.Store(int32(lvl))
}

type Logger struct {
	*zap.Logger
	level        *zap.AtomicLevel
	cm           config.Manager
	otelLevel    *levelFilterCore
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

	// Create console core first
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.Lock(os.Stdout),
		atomicLevel,
	)

	// Check if OTEL logger provider is available and add OTEL core
	provider := global.GetLoggerProvider()
	var core zapcore.Core
	var otelLevelFilter *levelFilterCore
	if provider != nil {
		var otelCore zapcore.Core = otelzap.NewCore(
			DefaultTracerService,
			otelzap.WithAttributes(),
			otelzap.WithLoggerProvider(provider),
		)

		// Always wrap OTEL core with levelFilterCore
		otelLevelFilter = &levelFilterCore{
			Core: otelCore,
		}

		// Set initial OTEL log level based on configuration
		if cm != nil && cm.Config() != nil && cm.Config().Core.Observability.IsLoggingEnabled() {
			minLevel := mapLogLevel(cm.Config().Core.Observability.Logging.Level)
			otelLevelFilter.SetMinLevel(minLevel)
		} else {
			// When logging is disabled, set to highest level to effectively drop all logs
			otelLevelFilter.SetMinLevel(zapcore.FatalLevel + 1)
		}

		core = zapcore.NewTee(otelLevelFilter, consoleCore)
	} else {
		core = consoleCore
	}

	zapLogger := zap.New(core, zap.AddCaller())

	logger := &Logger{
		Logger:     zapLogger,
		level:      &atomicLevel,
		cm:         cm,
		otelLevel:  otelLevelFilter,
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

		// Update OTEL level filter if it exists
		if l.otelLevel != nil && l.cm.Config().Core.Observability.IsLoggingEnabled() {
			l.otelLevel.SetMinLevel(mapLogLevel(l.cm.Config().Core.Observability.Logging.Level))
		}
	}
}

func (l *Logger) Level() *zap.AtomicLevel {
	return l.level
}

func (l *Logger) wrap(logger *zap.Logger) *Logger {
	return &Logger{
		Logger:    logger,
		level:     l.level,
		cm:        l.cm,
		otelLevel: l.otelLevel,
	}
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
