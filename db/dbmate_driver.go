package db

import (
	"database/sql"
	"fmt"
	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"net/url"
)

func init() {
	dbmate.RegisterDriver(NewSQLiteMemoryDriver, "sqlitememory")
}

var _ dbmate.Driver = (*sqliteMemoryDriver)(nil)

// sqliteMemoryDriver extends the standard SQLite driver with in-memory support
type sqliteMemoryDriver struct {
	*sqlite.Driver // Embed the standard driver for potential future use or shared methods
	config         dbmate.DriverConfig
	url            *url.URL // Store URL ourselves
}

// isMemoryDatabase checks if this driver is configured for an in-memory database
func (drv *sqliteMemoryDriver) isMemoryDatabase() bool {
	return drv.url.Host == "memory"
}

// DriverConfig returns the driver configuration
func (drv *sqliteMemoryDriver) DriverConfig() dbmate.DriverConfig {
	return drv.config
}

// NewSQLiteMemoryDriver initializes a new in-memory SQLite driver instance
func NewSQLiteMemoryDriver(config dbmate.DriverConfig) dbmate.Driver {
	// Create base driver with the config - although we won't use its Open/DatabaseExists directly for memory
	baseDriver := sqlite.NewDriver(config)

	return &sqliteMemoryDriver{
		Driver: baseDriver.(*sqlite.Driver), // Still embed for potential shared methods
		config: config,
		url:    config.DatabaseURL, // Store the URL from config
	}
}

// Open connects to an in-memory SQLite database
func (drv *sqliteMemoryDriver) Open() (*sql.DB, error) {
	if !drv.isMemoryDatabase() {
		return nil, fmt.Errorf("sqlitememory driver only supports in-memory databases")
	}
	return sql.Open("sqlite3", ":memory:")
}

// DatabaseExists checks if the database exists. For in-memory, it always returns true.
// This method overrides the embedded sqlite.Driver's DatabaseExists.
func (drv *sqliteMemoryDriver) DatabaseExists() (bool, error) {
	// For in-memory databases, they always "exist" when the connection is opened.
	// No filesystem check is needed.
	if drv.isMemoryDatabase() {
		return true, nil
	}
	// This case should ideally not be reached for a driver registered as "sqlitememory",
	// but as a safeguard, we could potentially fall back or return an error.
	// Returning false and an error indicating unexpected state is safer.
	return false, fmt.Errorf("unexpected call to DatabaseExists for non-memory path in sqlitememory driver")
}

// CreateDatabase is a no-op for in-memory databases
func (drv *sqliteMemoryDriver) CreateDatabase() error {
	if !drv.isMemoryDatabase() {
		return fmt.Errorf("sqlitememory driver only supports in-memory databases")
	}
	return nil // No-op for in-memory
}

// DropDatabase is a no-op for in-memory databases
func (drv *sqliteMemoryDriver) DropDatabase() error {
	if !drv.isMemoryDatabase() {
		return fmt.Errorf("sqlitememory driver only supports in-memory databases")
	}
	return nil // No-op for in-memory
}

// DumpSchema is not supported for in-memory databases
func (drv *sqliteMemoryDriver) DumpSchema(db *sql.DB) ([]byte, error) {
	if !drv.isMemoryDatabase() {
		return nil, fmt.Errorf("sqlitememory driver only supports in-memory databases")
	}
	return nil, fmt.Errorf("schema dump not supported for in-memory SQLite")
}
