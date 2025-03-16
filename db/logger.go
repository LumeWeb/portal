// Package db provides database functionality for the portal application.
package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"

	"go.uber.org/zap"
	dbLogger "gorm.io/gorm/logger"
)

// DBLogger is a GORM logger implementation using zap.
// It implements the gorm.logger.Interface to provide structured logging
// for database operations.
type DBLogger struct {
	logger *zap.Logger
	level  *zap.AtomicLevel
}

var _ dbLogger.Interface = (*DBLogger)(nil)

var (
	// levels maps GORM log levels to zap atomic levels
	levels = map[dbLogger.LogLevel]zap.AtomicLevel{
		dbLogger.Silent: zap.NewAtomicLevelAt(zap.InfoLevel),
		dbLogger.Error:  zap.NewAtomicLevelAt(zap.ErrorLevel),
		dbLogger.Warn:   zap.NewAtomicLevelAt(zap.WarnLevel),
		dbLogger.Info:   zap.NewAtomicLevelAt(zap.InfoLevel),
	}
)

// LogMode sets the log level for the logger.
// It returns a new logger instance with the updated log level.
func (l DBLogger) LogMode(level dbLogger.LogLevel) dbLogger.Interface {
	if atomicLevel, ok := levels[level]; ok {
		l.level.SetLevel(atomicLevel.Level())
		return l
	}

	l.logger.Fatal("invalid log level", zap.Int("level", int(level)))
	return nil
}

// Info logs info level messages.
// It converts variadic arguments to zap fields for structured logging.
func (l DBLogger) Info(ctx context.Context, s string, i ...interface{}) {
	l.logger.Info(s, interfacesToFields(i...)...)
}

// Warn logs warning level messages.
// It converts variadic arguments to zap fields for structured logging.
func (l DBLogger) Warn(ctx context.Context, s string, i ...interface{}) {
	l.logger.Warn(s, interfacesToFields(i...)...)
}

// Error logs error level messages.
// It converts variadic arguments to zap fields for structured logging.
func (l DBLogger) Error(ctx context.Context, s string, i ...interface{}) {
	l.logger.Error(s, interfacesToFields(i...)...)
}

// Trace logs SQL queries with execution time and affected rows.
// It only logs at debug level and ignores ErrRecordNotFound errors.
func (l DBLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level.Level() <= zap.DebugLevel {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}

		sql, rowsAffected := fc()
		fields := []zap.Field{
			zap.String("sql", sql),
			zap.Int64("rows_affected", rowsAffected),
			zap.Duration("elapsed", time.Since(begin)),
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
		}
		l.logger.Debug("trace", fields...)
	}
}

// NewLogger creates a new DBLogger instance with the provided zap logger and level.
func NewLogger(zlog *zap.Logger, zlogLevel *zap.AtomicLevel) *DBLogger {
	return &DBLogger{logger: zlog, level: zlogLevel}
}

// interfacesToFields converts interface values to zap fields for structured logging.
// It uses the index as the field key for each value.
func interfacesToFields(i ...interface{}) []zap.Field {
	fields := make([]zap.Field, 0)
	for idx, v := range i {
		fields = append(fields, zap.Any(strconv.Itoa(idx), v))
	}
	return fields
}
