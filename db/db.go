// Package db provides database functionality for the portal application,
// including connection management, caching, and transaction handling.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/migrations"
	"go.uber.org/zap"

	"github.com/go-gorm/caches/v4"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

var sanitizerRegexes = []struct {
	re      *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`"<binary>"`), `"<uuid>"`},
	{regexp.MustCompile(`'<binary>'`), `'<uuid>'`},
	{regexp.MustCompile(`X'[0-9a-fA-F]+'`), `"<uuid>"`},
}

func sanitizeTracingQuery(query string) string {
	for _, s := range sanitizerRegexes {
		query = s.re.ReplaceAllString(query, s.replace)
	}
	return query
}

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

// SetupDBObservability configures OpenTelemetry tracing and Prometheus metrics for the database connection.
// It returns an error if any plugin cannot be registered.
func SetupDBObservability(ctx core.Context) error {
	db := ctx.DB()
	cfg := ctx.Config().Config()
	observabilityCfg := cfg.Core.Observability
	dbName := cfg.Core.DB.Name

	// Setup OpenTelemetry tracing if enabled
	if observabilityCfg.IsTracingEnabled() {
		// Add a query formatter to sanitize binary UUID values from traces
		// This prevents invalid UTF-8 errors when OTLP serializes query strings as protobuf
		if err := db.Use(tracing.NewPlugin(
			tracing.WithoutMetrics(),
			tracing.WithQueryFormatter(sanitizeTracingQuery),
		)); err != nil {
			return err
		}
	}

	// Setup Prometheus metrics if enabled
	if observabilityCfg.IsMetricsEnabled() {
		if err := setupDBMetrics(ctx, db, dbName, observabilityCfg.Metrics.RefreshInterval); err != nil {
			return err
		}
	}

	return nil
}

// dbStatsCollector wraps GORM DB stats as Prometheus gauges registered on the
// portal's core metrics registry, exposed through the existing /metrics endpoint.
type dbStatsCollector struct {
	maxOpenConnections prometheus.Gauge
	openConnections    prometheus.Gauge
	inUse              prometheus.Gauge
	idle               prometheus.Gauge
	waitCount          prometheus.Gauge
	waitDuration       prometheus.Gauge
	maxIdleClosed      prometheus.Gauge
	maxLifetimeClosed  prometheus.Gauge
	maxIdleTimeClosed  prometheus.Gauge
}

func newDBStatsCollector(dbName string) *dbStatsCollector {
	labels := prometheus.Labels{"db_name": dbName}
	return &dbStatsCollector{
		maxOpenConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_max_open_connections",
			Help:        "Maximum number of open connections to the database.",
			ConstLabels: labels,
		}),
		openConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_open_connections",
			Help:        "The number of established connections both in use and idle.",
			ConstLabels: labels,
		}),
		inUse: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_in_use",
			Help:        "The number of connections currently in use.",
			ConstLabels: labels,
		}),
		idle: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_idle",
			Help:        "The number of idle connections.",
			ConstLabels: labels,
		}),
		waitCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_wait_count",
			Help:        "The total number of connections waited for.",
			ConstLabels: labels,
		}),
		waitDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_wait_duration",
			Help:        "The total time blocked waiting for a new connection.",
			ConstLabels: labels,
		}),
		maxIdleClosed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_max_idle_closed",
			Help:        "The total number of connections closed due to SetMaxIdleConns.",
			ConstLabels: labels,
		}),
		maxLifetimeClosed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_max_lifetime_closed",
			Help:        "The total number of connections closed due to SetConnMaxLifetime.",
			ConstLabels: labels,
		}),
		maxIdleTimeClosed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "gorm_dbstats_max_idletime_closed",
			Help:        "The total number of connections closed due to SetConnMaxIdleTime.",
			ConstLabels: labels,
		}),
	}
}

func (c *dbStatsCollector) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		c.maxOpenConnections,
		c.openConnections,
		c.inUse,
		c.idle,
		c.waitCount,
		c.waitDuration,
		c.maxIdleClosed,
		c.maxLifetimeClosed,
		c.maxIdleTimeClosed,
	}
}

func (c *dbStatsCollector) update(stats sql.DBStats) {
	c.maxOpenConnections.Set(float64(stats.MaxOpenConnections))
	c.openConnections.Set(float64(stats.OpenConnections))
	c.inUse.Set(float64(stats.InUse))
	c.idle.Set(float64(stats.Idle))
	c.waitCount.Set(float64(stats.WaitCount))
	c.waitDuration.Set(float64(stats.WaitDuration.Seconds()))
	c.maxIdleClosed.Set(float64(stats.MaxIdleClosed))
	c.maxLifetimeClosed.Set(float64(stats.MaxLifetimeClosed))
	c.maxIdleTimeClosed.Set(float64(stats.MaxIdleTimeClosed))
}

// setupDBMetrics registers DB stats gauges on the portal's core metrics registry
// and starts a background goroutine to refresh them at the configured interval.
func setupDBMetrics(ctx core.Context, db *gorm.DB, dbName string, refreshInterval uint) error {
	collector := newDBStatsCollector(dbName)
	for _, c := range collector.collectors() {
		if err := core.RegisterCoreCollector(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return fmt.Errorf("failed to register DB stats collector: %w", err)
			}
		}
	}

	go func() {
		ticker := time.NewTicker(time.Duration(refreshInterval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			sqlDB, err := db.DB()
			if err != nil {
				ctx.Logger().Error("Failed to get DB for stats collection", zap.Error(err))
				continue
			}
			collector.update(sqlDB.Stats())
		}
	}()

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
func RetryableTransaction(ctx context.Context, db *gorm.DB, operation func(tx *gorm.DB) *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return RetryOnLock(tx, func(tx *gorm.DB) *gorm.DB {
			return operation(tx)
		})
	})
}

// RetryableComponentTransaction executes a database transaction with retry logic for lock errors.
// It extracts the context and database connection from the provided component and delegates
// to RetryableTransaction for the actual execution.
//
// The component must implement the core.Component interface with methods:
// - Ctx() core.Context to get the execution context
// - DB() *gorm.DB to get the database connection
//
// Parameters:
//   - component: The component providing context and database connection
//   - operation: A function that takes a transaction *gorm.DB and returns a *gorm.DB result
//
// Returns:
//   - error: Any error encountered during transaction execution or retries
func RetryableComponentTransaction(component core.Component, ctx context.Context, operation func(tx *gorm.DB) *gorm.DB) error {
	return RetryableTransaction(ctx, component.DB(), operation)
}

// RetryableComponentLock executes a database operation with exponential backoff retry logic
// when database lock errors are encountered, using the component's database connection.
//
// The component must implement the core.Component interface with a DB() method that returns
// a *gorm.DB connection.
//
// Parameters:
//   - component: The component providing the database connection
//   - operation: A function that takes a *gorm.DB and returns a *gorm.DB result
//
// Returns:
//   - error: Any error encountered during operation execution or retries
func RetryableComponentLock(component core.Component, operation func(tx *gorm.DB) *gorm.DB) error {
	return RetryOnLock(component.DB(), operation)
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
