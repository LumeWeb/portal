// Package db provides database functionality for the portal application,
// including connection management, caching, and transaction handling.
package db

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/migrations"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"go.uber.org/zap"

	"github.com/go-gorm/caches/v4"
	"gorm.io/gorm"
)

// NewDatabase creates a new database connection and returns it along with context options.
// It configures the database based on the application configuration and sets up caching if enabled.
func NewDatabase(ctx core.Context) (*gorm.DB, []core.ContextBuilderOption) {
	cfg := ctx.Config()
	rootLogger := ctx.Logger()

	// Create a provider based on configuration
	provider := NewProvider(cfg)

	// Connect to the database
	db, err := provider.Connect(rootLogger)
	if err != nil {
		panic(err)
	}

	// Configure caching if needed
	cacher := getCacher(cfg, rootLogger)
	if cacher != nil {
		cache := &caches.Caches{Conf: &caches.Config{
			Cacher: cacher,
		}}
		err := db.Use(cache)
		if err != nil {
			rootLogger.Error("Failed to configure DB cache", zap.Error(err))
		}
	}

	// Create context options
	ctxOpts := []core.ContextBuilderOption{
		core.ContextWithDB(db),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return provider.Close()
		}),
	}

	return db, ctxOpts
}

// getCacheMode returns the cache mode from configuration.
// It handles default values and validates the mode.
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

// These functions are now handled by the provider implementations

// getCacher returns a caches.Cacher implementation based on the configured cache mode.
// It returns nil if caching is disabled.
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

// RetryOnLock executes a database operation with exponential backoff retry logic
// when database lock errors are encountered.
func RetryOnLock(db *gorm.DB, operation func(*gorm.DB) *gorm.DB) error {
	initialBackoff := 100 * time.Millisecond
	maxBackoff := 10 * time.Second
	attempt := 0

	for {
		result := operation(db)

		if result == nil {
			// Get caller information
			pc, file, line, ok := runtime.Caller(1)
			if !ok {
				log.Println("Unable to get caller information")
			} else {
				fn := runtime.FuncForPC(pc)
				log.Printf("Error in RetryOnLock called from %s (%s:%d)", fn.Name(), filepath.Base(file), line)
			}

			panic("operation must return a result")
		}

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

// RetryableTransaction executes a database transaction with retry logic for lock errors.
// It combines the transaction with the RetryOnLock functionality.
func RetryableTransaction(ctx core.Context, db *gorm.DB, operation func(*gorm.DB) *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return RetryOnLock(tx, func(tx *gorm.DB) *gorm.DB {
			return operation(tx)
		})
	})
}

// isLockError checks if the given error is a database lock error.
// It examines the error message for common lock-related patterns.
func isLockError(err error) bool {
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "deadlock") ||
		strings.Contains(errMsg, "lock wait timeout") ||
		strings.Contains(errMsg, "database is locked") ||
		strings.Contains(errMsg, "too many connections")
}

// GetCoreMigrations returns the core database migrations for different database types.
func GetCoreMigrations() core.DBMigration {
	return map[core.DBType]fs.FS{
		core.DB_TYPE_MYSQL:  migrations.GetMySQL(),
		core.DB_TYPE_SQLITE: migrations.GetSQLite(),
	}
}

// GetSQLiteDBFile returns the full path to the SQLite database file.
// It handles both absolute and relative paths.
func GetSQLiteDBFile(config config.Manager) string {
	dbCfg := config.Config().Core.DB
	if dbCfg.Type != "sqlite" {
		return ""
	}

	if path.IsAbs(dbCfg.File) {
		return dbCfg.File
	}

	return path.Join(config.ConfigDir(), dbCfg.File)
}
