// Package db provides database testing utilities.
package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"go.lumeweb.com/portal/core"
	coretesting "go.lumeweb.com/portal/core/testing"
	"gorm.io/gorm"
	"testing"
)

// NewSQLMock creates a new SQL mock for testing.
// It returns a configured GORM DB instance and the sqlmock interface for setting expectations.
func NewSQLMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	
	// Create a mock provider
	provider, mock, err := NewMockProvider(t)
	if err != nil {
		t.Fatalf("failed to create mock database provider: %v", err)
	}
	
	// Create a test logger
	logger := core.NewLogger(nil)
	
	// Connect to the mock database
	db, err := provider.Connect(logger)
	if err != nil {
		t.Fatalf("failed to create gorm database: %v", err)
	}
	
	// Register cleanup
	t.Cleanup(func() {
		_ = provider.Close()
	})
	
	return db, mock
}

// SetupSQLMock creates a new sqlmock and configures a test context with it.
// It returns a test context with the mock database and the sqlmock interface.
func SetupSQLMock(t *testing.T) (coretesting.TestContext, sqlmock.Sqlmock) {
	// Create a mock database and gorm instance
	mockDB, mock := NewSQLMock(t)
	
	// Create the test context with the mock DB
	ctx := coretesting.NewTestContext(t, coretesting.WithMockDB(mockDB))
	
	return ctx, mock
}
