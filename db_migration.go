package portal

import (
	"context"
	"errors"
	"fmt"
	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/mysql"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io/fs"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	migrationLockKey = "/discovery/portal/migrations/lock"
	migrationLockTTL = 5 * time.Minute // Generous timeout for migrations
)

type MigrationManager struct {
	ctx     core.Context
	etcdMgr *config.EtcdManager
	logger  *core.Logger
}

func NewMigrationManager(ctx core.Context) (*MigrationManager, error) {
	if !ctx.Config().Config().Core.ClusterEnabled() {
		return &MigrationManager{
			ctx:    ctx,
			logger: ctx.Logger(),
		}, nil
	}

	etcdManager, err := ctx.Config().Config().Core.Clustered.Etcd.GetManager(ctx.Logger().Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get etcd manager: %w", err)
	}

	return &MigrationManager{
		ctx:     ctx,
		etcdMgr: etcdManager,
		logger:  ctx.Logger(),
	}, nil
}

func (m *MigrationManager) RunMigrations(db *gorm.DB) error {
	// Only attempt migrations in cluster mode
	if !m.ctx.Config().Config().Core.ClusterEnabled() {
		return m.executeMigrations(db)
	}

	// Try to acquire migration lock
	lease, err := m.acquireMigrationLock()
	if err != nil {
		if errors.Is(err, ErrLockAcquireFailed) {
			m.logger.Info("Another instance is handling migrations, skipping...")
			return nil
		}
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer lease.Close()

	return m.executeMigrations(db)
}

func (m *MigrationManager) acquireMigrationLock() (*etcdLease, error) {
	// Create lease
	client := m.etcdMgr.Client()
	resp, err := client.Grant(context.Background(), int64(migrationLockTTL.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}

	// Try to acquire lock using lease
	txn := m.etcdMgr.Client().Txn(context.Background())
	txn = txn.If(clientv3.Compare(clientv3.CreateRevision(migrationLockKey), "=", 0))
	txn = txn.Then(clientv3.OpPut(migrationLockKey, "", clientv3.WithLease(resp.ID)))
	txn = txn.Else(clientv3.OpGet(migrationLockKey))

	txnResp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to execute transaction: %w", err)
	}

	if !txnResp.Succeeded {
		return nil, ErrLockAcquireFailed
	}

	// Create lease keeper
	lease := &etcdLease{
		client: m.etcdMgr.Client(),
		id:     resp.ID,
		logger: m.logger,
		done:   make(chan struct{}),
	}

	// Start lease keepalive
	go lease.keepalive()

	return lease, nil
}

func (m *MigrationManager) executeMigrations(_ *gorm.DB) error {
	m.logger.Info("Starting database migrations")

	m.logger.Debug("Running dbmate migrations")

	cfg := m.ctx.Config()
	dbConfig := cfg.Config().Core.DB
	dbType := dbConfig.Type

	compositFs := newCompositeFS()
	compositFs.Mount("0_core", getMigrationsByType(dbType, db.GetCoreMigrations()))

	pluginMigrations, migrationOrder, err := getMigrations()
	if err != nil {
		return err
	}

	for idx, plugin := range migrationOrder {
		migrations := getMigrationsByType(dbType, pluginMigrations[plugin])
		if migrations == nil {
			continue
		}
		compositFs.Mount(fmt.Sprintf("%d_%s", idx+1, plugin), migrations)
	}

	dbUrl, err := getDbMateUrl(cfg)
	if err != nil {
		return err
	}

	dbMateMigration := dbmate.New(dbUrl)
	dbMateMigration.FS = compositFs
	dbMateMigration.MigrationsDir = compositFs.Mounts()
	dbMateMigration.AutoDumpSchema = false

	err = dbMateMigration.CreateAndMigrate()
	if err != nil {
		return err
	}

	m.logger.Info("Database migrations completed successfully")

	return nil
}

// etcdLease handles lease keepalive and cleanup
type etcdLease struct {
	client *clientv3.Client
	id     clientv3.LeaseID
	logger *core.Logger
	done   chan struct{}
}

func (l *etcdLease) keepalive() {
	// Get the keep alive channel
	ch, err := l.client.KeepAlive(context.Background(), l.id)
	if err != nil {
		l.logger.Error("Failed to setup lease keepalive", zap.Error(err))
		return
	}

	for {
		select {
		case <-l.done:
			return
		case resp := <-ch:
			if resp == nil {
				l.logger.Error("Lease keepalive channel closed")
				return
			}
		}
	}
}

func (l *etcdLease) Close() {
	close(l.done)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Revoke lease
	_, err := l.client.Revoke(ctx, l.id)
	if err != nil {
		l.logger.Error("Failed to revoke lease", zap.Error(err))
	}
}

// Helper to get all models that need migration
func getModels(ctx core.Context) ([]interface{}, error) {
	plugins := core.GetPlugins()

	models := make([]interface{}, 0)
	for _, plugin := range plugins {
		if plugin.Models != nil && len(plugin.Models) > 0 {
			for _, model := range plugin.Models {
				typ := reflect.TypeOf(model)
				if typ.Kind() != reflect.Ptr {
					ctx.Logger().Error("Model must be a pointer", zap.String("model", typ.Name()))
					return nil, core.ErrInvalidModel
				}
			}
			models = append(models, plugin.Models...)
		}
	}

	// Add plugin models
	for _, plugin := range core.GetPlugins() {
		models = append(models, plugin.Models...)
	}

	return models, nil
}

func getMigrations() (map[string]core.DBMigration, []string, error) {
	plugins := core.GetPlugins()

	migrations := make(map[string]core.DBMigration)
	order := make([]string, 0)
	for _, plugin := range plugins {
		if plugin.Migrations != nil && len(plugin.Migrations) > 0 {
			migrations[plugin.ID] = plugin.Migrations
			order = append(order, plugin.ID)
		}
	}

	return migrations, order, nil
}

func getMigrationsByType(typ string, migrations core.DBMigration) fs.FS {
	switch typ {
	case "sqlite":
		return migrations["sqlite"]
	case "mysql":
		return migrations["mysql"]
	default:
		return nil
	}
}

// getDbMateUrl generates a database connection URL for dbmate based on the provided configuration.
// Returns a *url.URL object and any error that occurred during URL generation.
func getDbMateUrl(cfg config.Manager) (*url.URL, error) {
	databaseConfig := cfg.Config().Core.DB
	if databaseConfig.Type == "" {
		return nil, errors.New("database type is required")
	}

	var urlStr string

	switch strings.ToLower(databaseConfig.Type) {
	case "sqlite":
		if databaseConfig.File == "" {
			return nil, errors.New("sqlite database requires a file path")
		}
		urlStr = "sqlite://" + db.GetSQLiteDBFile(cfg)

	case "mysql":
		if databaseConfig.Host == "" {
			return nil, errors.New("mysql database requires a host")
		}
		if databaseConfig.Name == "" {
			return nil, errors.New("mysql database requires a database name")
		}

		// For MySQL, dbmate expects the format:
		// mysql://username:password@host:port/dbname?param1=value1
		// NOT using the tcp() wrapper that's used in Go's sql.Open

		// Handle default port if not specified
		port := databaseConfig.Port
		if port == 0 {
			port = 3306 // Default MySQL port
		}

		// Build query parameters
		params := make(url.Values)

		// Add charset if specified
		if databaseConfig.Charset != "" {
			params.Add("charset", databaseConfig.Charset)
		}

		// Add TLS parameters if enabled
		if databaseConfig.TLSEnabled {
			if databaseConfig.TLSSkipVerify {
				params.Add("tls", "skip-verify")
			} else {
				params.Add("tls", "true")
			}
		}

		// Create a standard URL that url.Parse can handle
		// Format: mysql://username:password@host:port/dbname?params
		u := &url.URL{
			Scheme:   "mysql",
			User:     url.UserPassword(databaseConfig.Username, databaseConfig.Password),
			Host:     fmt.Sprintf("%s:%d", databaseConfig.Host, port),
			Path:     "/" + databaseConfig.Name,
			RawQuery: params.Encode(),
		}

		return u, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", databaseConfig.Type)
	}

	// For SQLite, we need to parse the URL string
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	return parsedURL, nil
}

var ErrLockAcquireFailed = errors.New("failed to acquire migration lock")
