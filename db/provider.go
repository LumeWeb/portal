// Package db provides database functionality for the portal application.
package db

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"gorm.io/gorm"
)

// Provider defines an interface for database connection providers.
// Different database implementations (MySQL, SQLite, etc.) must implement this interface.
type Provider interface {
	// Connect establishes a connection to the database and returns a configured GORM DB instance.
	Connect(logger *core.Logger) (*gorm.DB, error)
	// GetDialector returns the appropriate GORM dialector for the database type.
	GetDialector() gorm.Dialector
	// Close closes the database connection and releases resources.
	Close() error
}

// NewProvider creates a database provider based on configuration.
// It returns the appropriate provider implementation based on the database type in the config.
func NewProvider(cfg config.Manager) Provider {
	dbType := cfg.Config().Core.DB.Type
	
	switch dbType {
	case "mysql":
		return NewMySQLProvider(cfg)
	case "sqlite":
		return NewSQLiteProvider(cfg)
	default:
		// Default to SQLite for safety
		return NewSQLiteProvider(cfg)
	}
}
