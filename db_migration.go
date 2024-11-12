package portal

import (
	"context"
	"errors"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"reflect"
	"time"
)

const (
	migrationLockKey = "/discovery/portal/migrations/lock"
	migrationLockTTL = 5 * time.Minute // Generous timeout for migrations
)

type MigrationManager struct {
	ctx        core.Context
	etcdClient *clientv3.Client
	logger     *core.Logger
}

func NewMigrationManager(ctx core.Context) (*MigrationManager, error) {
	if !ctx.Config().Config().Core.ClusterEnabled() {
		return &MigrationManager{
			ctx:    ctx,
			logger: ctx.Logger(),
		}, nil
	}

	client, err := ctx.Config().Config().Core.Clustered.Etcd.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &MigrationManager{
		ctx:        ctx,
		etcdClient: client,
		logger:     ctx.Logger(),
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
	resp, err := m.etcdClient.Grant(context.Background(), int64(migrationLockTTL.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}

	// Try to acquire lock using lease
	txn := m.etcdClient.Txn(context.Background())
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
		client: m.etcdClient,
		id:     resp.ID,
		logger: m.logger,
		done:   make(chan struct{}),
	}

	// Start lease keepalive
	go lease.keepalive()

	return lease, nil
}

func (m *MigrationManager) executeMigrations(db *gorm.DB) error {
	m.logger.Info("Starting database migrations")

	m.logger.Debug("Running GORM auto-migrations")

	models, err := getModels(m.ctx)
	if err != nil {
		return err
	}

	for _, model := range models {
		typ := reflect.TypeOf(model)
		// Get the underlying type if it's a pointer
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		if err = db.AutoMigrate(model); err != nil {
			m.logger.Error("Error migrating model", zap.String("model", typ.Name()), zap.Error(err))
			return err
		}
	}

	migrations, err := getMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if err = migration(db); err != nil {
			m.logger.Error("Error running migration", zap.Error(err))
			return err
		}
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
	ticker := time.NewTicker(migrationLockTTL / 3)
	defer ticker.Stop()

	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			_, err := l.client.KeepAliveOnce(context.Background(), l.id)
			if err != nil {
				l.logger.Error("Failed to keep lease alive", zap.Error(err))
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

func getMigrations() ([]core.DBMigration, error) {
	plugins := core.GetPlugins()

	migrations := make([]core.DBMigration, 0)
	for _, plugin := range plugins {
		if plugin.Migrations != nil && len(plugin.Migrations) > 0 {
			migrations = append(migrations, plugin.Migrations...)
		}
	}

	return migrations, nil
}

var ErrLockAcquireFailed = errors.New("failed to acquire migration lock")
