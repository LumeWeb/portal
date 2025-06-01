package db

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestSQLiteProvider implements db.Provider for in-memory SQLite testing
type TestSQLiteProvider struct {
	sqlDB  *gorm.DB
	cfg    *mocks.MockManager
	logger *core.Logger
}

// NewTestSQLiteProvider creates a new in-memory SQLite provider for testing
func NewTestSQLiteProvider() *TestSQLiteProvider {
	return &TestSQLiteProvider{
		cfg: mocks.NewMockManager(nil),
	}
}

// Connect establishes an in-memory SQLite connection
func (p *TestSQLiteProvider) Connect(logger *core.Logger) (*gorm.DB, error) {
	p.logger = logger

	_db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: db.NewLogger(logger.Logger, logger.Level()),
	})
	if err != nil {
		return nil, err
	}

	p.sqlDB = _db
	return _db, nil
}

// GetDialector returns the SQLite dialector configured for in-memory
func (p *TestSQLiteProvider) GetDialector() gorm.Dialector {
	return sqlite.Open("file::memory:?cache=shared")
}

// Close closes the database connection
func (p *TestSQLiteProvider) Close() error {
	if p.sqlDB != nil {
		sqlDB, err := p.sqlDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
