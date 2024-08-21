package db

import (
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"math"
	"math/rand/v2"
	"path"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"go.uber.org/zap"

	"github.com/go-gorm/caches/v4"
	"gorm.io/gorm"
)

func NewDatabase(ctx core.Context) (*gorm.DB, []core.ContextBuilderOption) {
	cfg := ctx.Config()
	rootLogger := ctx.Logger()

	dbType := cfg.Config().Core.DB.Type
	var db *gorm.DB
	var err error

	switch dbType {
	case "mysql":
		db, err = openMySQLDatabase(cfg, rootLogger)
	case "sqlite":
		var dbFile string

		if path.IsAbs(cfg.Config().Core.DB.File) {
			dbFile = cfg.Config().Core.DB.File
		} else {
			dbFile = path.Join(cfg.ConfigDir(), cfg.Config().Core.DB.File)
		}

		db, err = openSQLiteDatabase(dbFile, rootLogger)
	default:
		panic(fmt.Sprintf("unsupported database type: %s", dbType))
	}

	if err != nil {
		panic(err)
	}

	cacher := getCacher(cfg, rootLogger)
	if cacher != nil {
		cache := &caches.Caches{Conf: &caches.Config{
			Cacher: cacher,
		}}
		err := db.Use(cache)
		if err != nil {
			return nil, nil
		}
	}

	ctxOpts := []core.ContextBuilderOption{
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			return db.AutoMigrate(models.GetModels()...)
		}),
		core.ContextWithDB(db),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.Close()
		}),
	}

	return db, ctxOpts
}

func getCacheMode(cm config.Manager, logger *core.Logger) string {
	if cm.Config().Core.DB.Cache == nil {
		return "none"
	}

	switch cm.Config().Core.DB.Cache.Mode {
	case "", "none":
		return "none"
	case "memory":
		return "memory"
	case "redis":
		return "redis"
	default:
		logger.Fatal("invalid cache mode", zap.String("mode", string(cm.Config().Core.DB.Cache.Mode)))
	}

	return "none"
}

func openMySQLDatabase(cfg config.Manager, rootLogger *core.Logger) (*gorm.DB, error) {
	username := cfg.Config().Core.DB.Username
	password := cfg.Config().Core.DB.Password
	host := cfg.Config().Core.DB.Host
	port := cfg.Config().Core.DB.Port
	dbname := cfg.Config().Core.DB.Name
	charset := cfg.Config().Core.DB.Charset

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local", username, password, host, port, dbname, charset)

	return gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger(rootLogger.Logger, rootLogger.Level()),
	})
}

func openSQLiteDatabase(file string, rootLogger *core.Logger) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(file), &gorm.Config{
		Logger: newLogger(rootLogger.Logger, rootLogger.Level()),
	})
}

func getCacher(cm config.Manager, logger *core.Logger) caches.Cacher {
	mode := getCacheMode(cm, logger)

	switch mode {
	case "none":
		return nil

	case "memory":
		return &memoryCacher{}
	case "redis":
		rcfg, ok := cm.Config().Core.DB.Cache.Options.(*config.RedisConfig)
		if !ok {
			logger.Fatal("invalid redis config")
			return nil
		}
		return &redisCacher{
			redis.NewClient(&redis.Options{
				Addr:     rcfg.Address,
				Password: rcfg.Password,
				DB:       rcfg.DB,
			}),
		}
	}

	return nil
}
func RetryOnLock(db *gorm.DB, operation func(*gorm.DB) *gorm.DB) error {
	initialBackoff := 100 * time.Millisecond
	maxBackoff := 10 * time.Second
	attempt := 0

	for {
		result := operation(db)
		if result.Error == nil {
			return nil
		}

		if !isLockError(result.Error) {
			return result.Error
		}

		backoff := float64(initialBackoff) * math.Pow(2, float64(attempt))
		jitter := rand.Float64() * float64(initialBackoff)
		sleepDuration := time.Duration(math.Min(backoff+jitter, float64(maxBackoff)))
		time.Sleep(sleepDuration)
		attempt++
	}
}

func RetryableTransaction(ctx core.Context, db *gorm.DB, operation func(*gorm.DB) *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return RetryOnLock(tx, func(tx *gorm.DB) *gorm.DB {
			return operation(tx)
		})
	})
}

// isLockError checks if the given error is a database lock error
func isLockError(err error) bool {
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "deadlock") ||
		strings.Contains(errMsg, "lock wait timeout") ||
		strings.Contains(errMsg, "database is locked") ||
		strings.Contains(errMsg, "too many connections")
}
