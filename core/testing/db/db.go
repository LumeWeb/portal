// Package db provides database testing utilities.
package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.lumeweb.com/portal/core"
	"gorm.io/gorm"
)

// NewSQLMock creates a new SQL mock for testing.
// It returns a configured GORM DB instance and the sqlmock interface for setting expectations.
func NewSQLMock(t testing.TB) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	// Create a mock provider
	provider, mock, err := NewMockProvider(t.(*testing.T))
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
