package db

import (
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/migrations"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"net/url"
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
		db, err = openSQLiteDatabase(GetSQLiteDBFile(cfg), rootLogger)
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
	dbConfig := cfg.Config().Core.DB

	// Build query parameters
	query := url.Values{}
	query.Set("parseTime", "True")
	query.Set("loc", "Local")

	if dbConfig.TLSEnabled {
		rootLogger.Debug("TLS enabled")
		if dbConfig.TLSSkipVerify {
			rootLogger.Debug("Skipping TLS verification")
			query.Set("tls", "skip-verify")
		} else {
			query.Set("tls", "true")
		}
	}

	if dbConfig.Charset != "" {
		query.Set("charset", dbConfig.Charset)
		rootLogger.Debug("Setting charset", zap.String("charset", dbConfig.Charset))
	} else {
		rootLogger.Debug("Charset is empty, skipping parameter")
	}

	// Construct the DSN using url.URL
	u := &url.URL{
		Scheme:   "tcp",
		User:     url.UserPassword(dbConfig.Username, dbConfig.Password),
		Host:     fmt.Sprintf("%s:%d", dbConfig.Host, dbConfig.Port),
		Path:     dbConfig.Name,
		RawQuery: query.Encode(),
	}

	// Decode the user info portion since MySQL doesn't expect it to be URL encoded
	userStr, err := url.QueryUnescape(u.User.String())
	if err != nil {
		return nil, fmt.Errorf("failed to unescape user string: %v", err)
	}

	// Format as MySQL DSN with the decoded user string
	dsn := fmt.Sprintf("%s@%s(%s)/%s?%s",
		userStr,
		u.Scheme,
		u.Host,
		strings.TrimPrefix(u.Path, "/"),
		u.RawQuery,
	)

	rootLogger.Debug("Connecting to MySQL", zap.String("dsn", dsn))

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

func GetCoreMigrations() core.DBMigration {
	return map[core.DBType]fs.FS{
		core.DB_TYPE_MYSQL:  migrations.GetMySQL(),
		core.DB_TYPE_SQLITE: migrations.GetSQLite(),
	}
}

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
