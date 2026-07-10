// Package db provides database functionality for the portal application.
package db

import (
	"fmt"
	"time"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// MySQLProvider implements the db.Provider interface for MySQL databases.
type MySQLProvider struct {
	cfg   config.Manager // Configuration manager
	sqlDB *gorm.DB       // GORM database instance
}

// NewMySQLProvider creates a new MySQL provider with the specified configuration.
func NewMySQLProvider(cfg config.Manager) *MySQLProvider {
	return &MySQLProvider{
		cfg: cfg,
	}
}

// Connect establishes a connection to the MySQL database.
// It configures the connection with the provided logger and returns a GORM DB instance.
func (p *MySQLProvider) Connect(logger *core.Logger) (*gorm.DB, error) {
	dsn, err := GetDSN(p.cfg)
	if err != nil {
		return nil, err
	}

	logger.Debug("Connecting to MySQL", zap.String("dsn", dsn))

	dbConfig := p.cfg.Config().Core.DB

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:      NewLogger(logger.Logger, logger.Level()),
		NowFunc:     func() time.Time { return time.Now().UTC() },
		PrepareStmt: true,
	})

	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if dbConfig.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(dbConfig.MaxOpenConns)
	}
	if dbConfig.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(dbConfig.MaxIdleConns)
	}
	if dbConfig.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(dbConfig.ConnMaxLifetime)
	}

	p.sqlDB = db
	return db, nil
}

// GetDialector returns the MySQL dialector with a simplified DSN.
// This is used when the dialector is needed without establishing a connection.
func (p *MySQLProvider) GetDialector() gorm.Dialector {
	dbConfig := p.cfg.Config().Core.DB

	// Build DSN (simplified for dialector)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Name)

	return mysql.Open(dsn)
}

// Close closes the database connection and releases resources.
func (p *MySQLProvider) Close() error {
	if p.sqlDB != nil {
		sqlDB, err := p.sqlDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
