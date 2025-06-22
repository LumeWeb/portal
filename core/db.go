package core

import (
	"github.com/pressly/goose/v3"
	"io/fs"
	"path/filepath"
	"runtime"
)

// DBType represents the type of database being used
type DBType string

const (
	// DB_TYPE_MYSQL represents a MySQL database
	DB_TYPE_MYSQL DBType = "mysql"
	
	// DB_TYPE_SQLITE represents a SQLite database
	DB_TYPE_SQLITE DBType = "sqlite"
)

// DBMigration holds filesystem migrations for different database types.
// The map key is the DBType and the value is the migration files for that database.
type DBMigration map[DBType]fs.FS

// GoDBMigration is an alias for goose.GoMigrationContext representing a Go migration context
type GoDBMigration = goose.GoMigrationContext

// RegisterDBMigration registers a new database migration with goose.
// The up function is executed when applying the migration, and down when rolling back.
// The migration is automatically named based on the caller's filename.
func RegisterDBMigration(up, down GoDBMigration) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("failed to get caller information for migration registration")
	}
	baseName := filepath.Base(filename)
	goose.AddNamedMigrationContext(baseName, up, down)
}
