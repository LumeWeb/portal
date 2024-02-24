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

var _ dbLogger.Interface = (*logger)(nil)

var (
	levels = map[dbLogger.LogLevel]zap.AtomicLevel{
		dbLogger.Silent: zap.NewAtomicLevelAt(zap.InfoLevel),
		dbLogger.Error:  zap.NewAtomicLevelAt(zap.ErrorLevel),
		dbLogger.Warn:   zap.NewAtomicLevelAt(zap.WarnLevel),
		dbLogger.Info:   zap.NewAtomicLevelAt(zap.InfoLevel),
	}
)

type logger struct {
	logger *zap.Logger
	level  *zap.AtomicLevel
}

func (l logger) LogMode(level dbLogger.LogLevel) dbLogger.Interface {
	if atomicLevel, ok := levels[level]; ok {
		l.level.SetLevel(atomicLevel.Level())
		return l
	}

	l.logger.Fatal("invalid log level", zap.Int("level", int(level)))
	return nil
}

func (l logger) Info(ctx context.Context, s string, i ...interface{}) {
	l.logger.Info(s, interfacesToFields(i...)...)
}

func (l logger) Warn(ctx context.Context, s string, i ...interface{}) {
	l.logger.Warn(s, interfacesToFields(i...)...)
}

func (l logger) Error(ctx context.Context, s string, i ...interface{}) {
	l.logger.Error(s, interfacesToFields(i...)...)
}

func (l logger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
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

func newLogger(zlog *zap.Logger, zlogLevel *zap.AtomicLevel) *logger {
	return &logger{logger: zlog, level: zlogLevel}
}

func interfacesToFields(i ...interface{}) []zap.Field {
	fields := make([]zap.Field, 0)
	for idx, v := range i {
		fields = append(fields, zap.Any(strconv.Itoa(idx), v))
	}
	return fields
}
