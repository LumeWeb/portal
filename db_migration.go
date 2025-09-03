package portal

import (
	"database/sql"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"unicode"

	"github.com/pressly/goose/v3"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GOOSE_TABLE_NAME_FORMAT defines the per-plugin goose version table name.
// %s is the (sanitized) plugin identifier.
const GOOSE_TABLE_NAME_FORMAT = "goose_%s_version"

// GOOSE_CORE_TABLE_NAME is the goose version table for core migrations.
const GOOSE_CORE_TABLE_NAME = "goose_db_version"

type MigrationManager struct {
	ctx    core.Context
	logger *core.Logger
}

func NewMigrationManager(ctx core.Context) (*MigrationManager, error) {
	return &MigrationManager{
		ctx:    ctx,
		logger: ctx.Logger(),
	}, nil
}

func normalizeDbDialect(dbType string) string {
	switch dbType {
	case "sqlite", "sqlitememory":
		return "sqlite3"
	case "mysql":
		return "mysql"
	default:
		return dbType
	}
}

func (m *MigrationManager) RunMigrations(db *gorm.DB) error {
	return m.ExecuteMigrations(db)
}

func (m *MigrationManager) ExecuteMigrations(_db *gorm.DB) error {
	m.logger.Info("Starting database migrations")

	cfg := m.ctx.Config()
	dbType := cfg.Config().Core.DB.Type

	sqlDb, err := _db.DB()
	if err != nil {
		return fmt.Errorf("failed to get db: %w", err)
	}

	// Run core migrations first
	m.logger.Info("Running core migrations", zap.String("db_type", dbType))
	if err := m.runCoreMigrations(sqlDb, dbType); err != nil {
		m.logger.Error("Failed to run core migrations", zap.Error(err))
		return err
	}
	m.logger.Info("Core migrations completed successfully")

	// Run each plugin's migrations individually
	m.logger.Info("Running plugin migrations", zap.String("db_type", dbType))
	if err := m.runPluginMigrations(sqlDb, dbType); err != nil {
		m.logger.Error("Failed to run plugin migrations", zap.Error(err))
		return err
	}
	m.logger.Info("Plugin migrations completed successfully")

	m.logger.Info("Database migrations completed successfully")
	return nil
}

func (m *MigrationManager) runCoreMigrations(sqlDb *sql.DB, dbType string) error {
	// Set the default table name for core migrations
	goose.SetTableName(GOOSE_CORE_TABLE_NAME)

	coreMigrations := db.GetCoreMigrations()
	coreFS := getMigrationsByType(dbType, coreMigrations)
	if coreFS == nil {
		m.logger.Warn("No core migrations found for database type", zap.String("db_type", dbType))
		return nil
	}

	m.logger.Debug("Setting up core migrations", zap.String("db_type", dbType))
	goose.SetBaseFS(coreFS)
	dialect := normalizeDbDialect(dbType)
	if err := goose.SetDialect(dialect); err != nil {
		m.logger.Error("Failed to select goose db dialect for core migrations",
			zap.String("db_type", dbType),
			zap.String("dialect", dialect),
			zap.Error(err))
		return fmt.Errorf("failed to select goose db dialect: %w", err)
	}

	m.logger.Debug("Running core migrations")
	if err := goose.RunContext(m.ctx.GetContext(), "up", sqlDb, "."); err != nil {
		m.logger.Error("Failed to run core migrations", zap.String("db_type", dbType), zap.Error(err))
		return fmt.Errorf("failed to run core migrations: %w", err)
	}

	return nil
}

func (m *MigrationManager) runPluginMigrations(sqlDb *sql.DB, dbType string) error {
	pluginMigrations, migrationOrder, err := getMigrations()
	if err != nil {
		m.logger.Error("Failed to get plugin migrations", zap.Error(err))
		return err
	}

	m.logger.Info("Found plugins with migrations", zap.Int("count", len(migrationOrder)))

	for _, plugin := range migrationOrder {
		tableName := pluginTableName(plugin)

		m.logger.Debug("Running migrations for plugin", zap.String("plugin", plugin), zap.String("table", tableName))

		// Set custom table name for this plugin
		goose.SetTableName(tableName)

		migrations := getMigrationsByType(dbType, pluginMigrations[plugin])
		if migrations == nil {
			m.logger.Debug("No migrations found for plugin", zap.String("plugin", plugin), zap.String("db_type", dbType))
			continue
		}

		m.logger.Debug("Setting up plugin migrations", zap.String("plugin", plugin), zap.String("db_type", dbType))
		goose.SetBaseFS(migrations)
		dialect := normalizeDbDialect(dbType)
		if err := goose.SetDialect(dialect); err != nil {
			m.logger.Error("Failed to select goose db dialect for plugin",
				zap.String("plugin", plugin),
				zap.String("db_type", dbType),
				zap.String("dialect", dialect),
				zap.Error(err))
			return fmt.Errorf("failed to select goose db dialect for plugin %s: %w", plugin, err)
		}

		m.logger.Debug("Running plugin migrations", zap.String("plugin", plugin))
		if err := goose.RunContext(m.ctx.GetContext(), "up", sqlDb, "."); err != nil {
			m.logger.Error("Failed to run migrations for plugin", zap.String("plugin", plugin), zap.Error(err))
			return fmt.Errorf("failed to run migrations for plugin %s: %w", plugin, err)
		}

		m.logger.Debug("Plugin migrations completed", zap.String("plugin", plugin))
		
		// Hygiene: clear per-plugin state to avoid leaking Goose globals.
		goose.SetBaseFS(nil)
		// Table name will be set again on next iteration; no need to restore here.
	}

	return nil
}

// Helper to get all models that need migration
func getModels(ctx core.Context) ([]interface{}, error) {
	plugins := core.GetPlugins()

	models := make([]interface{}, 0)
	for _, plugin := range plugins {
		if plugin.Models != nil && len(plugin.Models) > 0 {
			for _, model := range plugin.Models {
				typ := reflect.TypeOf(model)
				if typ.Kind() != reflect.Ptr {
					ctx.Logger().Error("Model must be a pointer", zap.String("model", typ.Name()))
					return nil, core.ErrInvalidModel
				}
			}
			models = append(models, plugin.Models...)
		}
	}

	return models, nil
}

func getMigrations() (map[string]core.DBMigration, []string, error) {
	plugins := core.GetPlugins()

	migrations := make(map[string]core.DBMigration)
	order := make([]string, 0)
	for _, plugin := range plugins {
		if plugin.Migrations != nil && len(plugin.Migrations) > 0 {
			migrations[plugin.ID] = plugin.Migrations
			order = append(order, plugin.ID)
		}
	}

	return migrations, order, nil
}

func getMigrationsByType(typ string, migrations core.DBMigration) fs.FS {
	switch typ {
	case "sqlite", "sqlitememory":
		return migrations[core.DB_TYPE_SQLITE]
	case "mysql":
		return migrations[core.DB_TYPE_MYSQL]
	default:
		return nil
	}
}

// sanitizeIdent converts a string to a valid SQL identifier by:
// 1. Lowercasing all characters
// 2. Replacing any character that is not [a-z0-9_] with '_'
// 3. Ensuring it doesn't start with a digit
// 4. Truncating to 64 characters (MySQL identifier limit)
func sanitizeIdent(ident string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return unicode.ToLower(r)
		case r >= '0' && r <= '9':
			return r
		case r == '_':
			return r
		default:
			return '_'
		}
	}, ident)

	if s == "" {
		s = "plugin"
	}

	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}

	// Truncate to 64 characters if needed
	if len(s) > 64 {
		s = s[:64]
	}

	return s
}

// pluginTableName generates a safe table name for goose migrations.
func pluginTableName(plugin string) string {
	const prefix = "goose_"
	const suffix = "_version"
	const maxLen = 64

	// Sanitize the plugin identifier
	sanitized := sanitizeIdent(plugin)

	// Calculate reserved length for prefix and suffix
	reservedLen := len(prefix) + len(suffix)

	// If the total length fits within the limit, return as-is
	if len(sanitized)+reservedLen <= maxLen {
		return fmt.Sprintf("%s%s%s", prefix, sanitized, suffix)
	}

	// Truncate the sanitized plugin name to fit within limits
	availableLen := maxLen - reservedLen
	truncated := sanitized[:availableLen]

	return fmt.Sprintf("%s%s%s", prefix, truncated, suffix)
}
