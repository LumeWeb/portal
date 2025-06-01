package db

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
)

// TestSQLiteProvider implements db.Provider for SQLite testing
type TestSQLiteProvider struct {
	*db.SQLiteProvider
}

// NewTestSQLiteProvider creates a new SQLite provider for testing
func NewTestSQLiteProvider(ctx core.Context) *TestSQLiteProvider {
	return &TestSQLiteProvider{
		SQLiteProvider: db.NewSQLiteProvider(ctx.Config()),
	}
}
