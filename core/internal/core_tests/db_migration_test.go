package core_tests

import (
	"errors"
	"go.lumeweb.com/portal"
	mockMigrations "go.lumeweb.com/portal/core/internal/core_tests/fixtures/migrations"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"testing"
	"testing/fstest"
)

func TestMigrationManager_RunMigrations(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a MigrationManager instance
		migrationManager, err := portal.NewMigrationManager(ctx)
		if err != nil {
			t.Fatalf("Failed to create MigrationManager: %v", err)
		}

		// Run migrations
		db := ctx.DB()
		err = migrationManager.RunMigrations(db)
		if err != nil {
			t.Fatalf("RunMigrations failed: %v", err)
		}

		// Verify migrations table exists
		var tableExists bool
		err = db.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='goose_db_version');").Scan(&tableExists).Error
		if err != nil {
			t.Fatalf("Failed to check migrations table: %v", err)
		}
		if !tableExists {
			t.Error("Migrations table 'goose_db_version' does not exist")
		}
	}, coreTesting.WithSQLite(t))
}

func TestMigrationManager_RunMigrations_NoCluster(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Disable cluster mode
		err := ctx.Config().Set(ctx, "core.clustered.enabled", false)
		if err != nil {
			t.Fatalf("Failed to disable cluster mode: %v", err)
		}

		// Create a MigrationManager instance
		migrationManager, err := portal.NewMigrationManager(ctx)
		if err != nil {
			t.Fatalf("Failed to create MigrationManager: %v", err)
		}

		// Run migrations
		db := ctx.DB()
		err = migrationManager.RunMigrations(db)
		if err != nil {
			t.Fatalf("RunMigrations failed: %v", err)
		}

		// Verify migrations table exists
		var tableExists bool
		err = db.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='goose_db_version');").Scan(&tableExists).Error
		if err != nil {
			t.Fatalf("Failed to check migrations table: %v", err)
		}
		if !tableExists {
			t.Error("Migrations table 'goose_db_version' does not exist")
		}
	}, coreTesting.WithSQLite(t))
}

func TestMigrationManager_RunMigrations_LockAcquireFailed(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Enable cluster mode
		err := ctx.Config().Set(ctx, "core.clustered.enabled", true)
		if err != nil {
			t.Fatalf("Failed to enable cluster mode: %v", err)
		}

		// Create a MigrationManager instance
		migrationManager, err := portal.NewMigrationManager(ctx)
		if err != nil {
			t.Fatalf("Failed to create MigrationManager: %v", err)
		}

		// Run migrations
		db := ctx.DB()
		err = migrationManager.RunMigrations(db)
		if err != nil && !errors.Is(err, portal.ErrLockAcquireFailed) {
			t.Fatalf("RunMigrations failed: %v", err)
		}

		// Verify migrations table exists
		var tableExists bool
		err = db.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='goose_db_version');").Scan(&tableExists).Error
		if err != nil {
			t.Fatalf("Failed to check migrations table: %v", err)
		}
		if !tableExists {
			t.Error("Migrations table 'goose_db_version' does not exist")
		}
	}, coreTesting.WithSQLite(t))
}

func TestMigrationManager_executeMigrations(t *testing.T) {
	// Define the migration file system
	migrationsFS := fstest.MapFS{
		"00001_test_migration.sql": &fstest.MapFile{
			Data: []byte("-- +goose Up\nCREATE TABLE test_table1 (id INTEGER PRIMARY KEY);\n\n-- +goose Down\nDROP TABLE test_table1;\n"),
		},
	}

	// Define the plugin ID
	pluginID := "testplugin"

	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a MigrationManager instance
		migrationManager, err := portal.NewMigrationManager(ctx)
		if err != nil {
			t.Fatalf("Failed to create MigrationManager: %v", err)
		}

		// Execute migrations
		db := ctx.DB()
		err = migrationManager.ExecuteMigrations(db)
		if err != nil {
			t.Fatalf("executeMigrations failed: %v", err)
		}

		// Add assertions here to check the state of the database after migrations
		var tableExists bool
		err = db.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='test_table1');").Scan(&tableExists).Error
		if err != nil {
			t.Fatalf("Failed to check if table exists: %v", err)
		}
		if !tableExists {
			t.Error("Table 'test_table' does not exist after migrations")
		}
	}, coreTesting.WithSQLite(t), coreTesting.WithSQLitePluginMigrations(pluginID, migrationsFS))
}

func TestMigrationManager_executeMigrations_GoMigration(t *testing.T) {
	// Define the plugin ID
	pluginID := "testplugin"

	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a MigrationManager instance
		migrationManager, err := portal.NewMigrationManager(ctx)
		if err != nil {
			t.Fatalf("Failed to create MigrationManager: %v", err)
		}

		// Execute migrations
		db := ctx.DB()
		err = migrationManager.ExecuteMigrations(db)
		if err != nil {
			t.Fatalf("executeMigrations failed: %v", err)
		}

		// Add assertions here to check the state of the database after migrations
		var tableExists bool
		err = db.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='test_table');").Scan(&tableExists).Error
		if err != nil {
			t.Fatalf("Failed to check if table exists: %v", err)
		}
		if !tableExists {
			t.Error("Table 'test_table' does not exist after migrations")
		}
	}, coreTesting.WithSQLite(t), coreTesting.WithSQLitePluginMigrations(pluginID, mockMigrations.GetSQLite()))
}
