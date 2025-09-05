// Package db provides database testing utilities.
package db

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.lumeweb.com/portal/core"
	dbCore "go.lumeweb.com/portal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockProvider implements db.Provider for testing purposes.
// It uses sqlmock to simulate database interactions without a real database.
type MockProvider struct {
	t            *testing.T            // Testing context
	sqlDB        *sql.DB               // Mock SQL database
	mock         sqlmock.Sqlmock       // SQL mock interface
	gormDB       *gorm.DB              // GORM database instance
	getDialector func() gorm.Dialector // Function to get the database dialector
}

// NewMockProvider creates a new mock database provider for testing.
// It returns the provider, the mock interface, and any error that occurred.
func NewMockProvider(t *testing.T) (*MockProvider, sqlmock.Sqlmock, error) {
	t.Helper()

	// Create a mock database connection
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}

	provider := &MockProvider{
		t:     t,
		sqlDB: mockDB,
		mock:  mock,
	}

	// Set default dialector function
	provider.getDialector = provider.defaultDialector

	return provider, mock, nil
}

// defaultDialector returns the default SQLite dialector configured for testing.
// It uses the mock SQL connection for database operations.
func (p *MockProvider) defaultDialector() gorm.Dialector {
	return sqlite.New(sqlite.Config{
		Conn: p.sqlDB,
	})
}

// Connect establishes a connection to the mock database.
// It configures the connection with the provided logger and returns a GORM DB instance.
func (p *MockProvider) Connect(logger *core.Logger) (*gorm.DB, error) {
	// Get the dialector (either default or custom)
	dialector := p.getDialector()

	// Create a gorm DB with the mock connection
	var dbConfig gorm.Config

	// If logger is provided, use it
	if logger != nil {
		dbConfig.Logger = dbCore.NewLogger(logger.Logger, logger.Level())
	}

	db, err := gorm.Open(dialector, &dbConfig)

	if err != nil {
		return nil, err
	}

	p.gormDB = db
	return db, nil
}

// GetDialector returns the current dialector function.
func (p *MockProvider) GetDialector() gorm.Dialector {
	return p.getDialector()
}

// SetDialector allows setting a custom dialector function.
// This enables testing with different database dialectors.
func (p *MockProvider) SetDialector(dialectorFunc func() gorm.Dialector) {
	p.getDialector = dialectorFunc
}

// Close closes the mock database connection.
func (p *MockProvider) Close() error {
	return p.sqlDB.Close()
}

// GetMock returns the underlying sqlmock interface.
// This allows tests to set expectations on database calls.
func (p *MockProvider) GetMock() sqlmock.Sqlmock {
	return p.mock
}
