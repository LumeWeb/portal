// Package testing provides utilities for testing core components
package testing

import (
	"fmt"
	"os"

	"github.com/DATA-DOG/go-sqlmock"
	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WithSQLite configures the test context to use a temporary file-backed SQLite database
func WithSQLite() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Generate temp file path
		tempFile, err := os.CreateTemp("", "portal-test-*.db")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp db file: %w", err)
		}
		err = tempFile.Close()
		if err != nil {
			return nil, err
		}

		// Update config with SQLite type and temp file path
		err = ctx.Config().Set(ctx, "core.db.type", "sqlite")
		if err != nil {
			return nil, fmt.Errorf("failed to set database type: %w", err)
		}
		err = ctx.Config().Set(ctx, "core.db.file", tempFile.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to set database file: %w", err)
		}

		// Add a startup function that will create and connect the DB
		startupOpt := core.ContextWithStartupFunc(func(ctx core.Context) error {
			provider := db.NewTestSQLiteProvider(ctx)
			connected := false
			
			// Connect to the database
			_db, err := provider.Connect(ctx.Logger())
			if err != nil {
				return fmt.Errorf("failed to connect to SQLite: %w", err)
			}
			connected = true

			// Set the DB on the context
			if tc, ok := ctx.(TestContext); ok {
				tc.SetDB(_db)
			} else {
				return fmt.Errorf("context is not TestContext")
			}

			// Close DB then remove temp file; always attempt removal, return close error.
			ctx.OnExit(func(c core.Context) error {
				closeErr := provider.Close()
				if err := os.Remove(tempFile.Name()); err != nil && !os.IsNotExist(err) {
					c.Logger().Error("Failed to remove temp SQLite file",
						zap.String("path", tempFile.Name()),
						zap.Error(err))
				}
				return closeErr
			})

			return nil
		})

		// Apply the startup option
		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

// WithMockDB adds a mock database to the test context
func WithMockDB(db *gorm.DB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if db == nil {
			return nil, fmt.Errorf("gorm.DB instance cannot be nil")
		}

		if tc, ok := ctx.(TestContext); ok {
			tc.SetDB(db)
		}

		ctx.RegisterCleanup(func() {
			sqlDB, err := db.DB()
			if err != nil {
				ctx.Logger().Error("failed to get sql.DB from gorm.DB", zap.Error(err))
				return
			}

			if sqlDB != nil {
				if closeErr := sqlDB.Close(); closeErr != nil {
					ctx.Logger().Error("failed to close sql.DB", zap.Error(closeErr))
				}
			}
		})

		return ctx, nil
	}
}

// SetupSQLMock creates a new sqlmock and configures a test context with it.
// It returns a test context with the mock database and the sqlmock interface.
func SetupSQLMock(t TB) (TestContext, sqlmock.Sqlmock) {
	// Create a mock database and gorm instance
	mockDB, _mock := db.NewSQLMock(t)

	// Create the test context with the mock DB
	ctx, err := NewTestContext(t, WithMockDB(mockDB))
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}

	return ctx, _mock
}

// WithDBMigrations adds a startup function that runs migrations if enabled
func WithDBMigrations() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		startupOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			if ShouldRunDBMigrations() {
				if coreCtx.DB() == nil {
					return fmt.Errorf("migrations enabled but no database connection available")
				}
				return RunMigrations(coreCtx.(TestContext))
			}
			return nil
		})
		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

// RunMigrations executes database migrations for the test context
func RunMigrations(ctx TestContext) error {
	migrationManager, err := portal.NewMigrationManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to create migration manager: %w", err)
	}

	if err = migrationManager.RunMigrations(ctx.DB()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
