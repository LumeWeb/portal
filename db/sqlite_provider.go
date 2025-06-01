// Package db provides database functionality for the portal application.
package db

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLiteProvider implements the db.Provider interface for SQLite databases.
type SQLiteProvider struct {
	cfg   config.Manager // Configuration manager
	sqlDB *gorm.DB       // GORM database instance
}

// NewSQLiteProvider creates a new SQLite provider with the specified configuration.
func NewSQLiteProvider(cfg config.Manager) *SQLiteProvider {
	return &SQLiteProvider{
		cfg: cfg,
	}
}

// Connect establishes a connection to the SQLite database.
// It configures the connection with the provided logger and returns a GORM DB instance.
func (p *SQLiteProvider) Connect(logger *core.Logger) (*gorm.DB, error) {
	dbFile, err := GetDSN(p.cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{
		Logger: NewLogger(logger.Logger, logger.Level()),
	})

	if err != nil {
		return nil, err
	}

	p.sqlDB = db
	return db, nil
}

// GetDialector returns the SQLite dialector with the configured database file.
// This is used when the dialector is needed without establishing a connection.
func (p *SQLiteProvider) GetDialector() gorm.Dialector {
	dbFile := GetSQLiteDBFile(p.cfg)
	return sqlite.Open(dbFile)
}

// Close closes the database connection and releases resources.
func (p *SQLiteProvider) Close() error {
	if p.sqlDB != nil {
		sqlDB, err := p.sqlDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
