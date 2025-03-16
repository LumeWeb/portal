// Package db provides database functionality for the portal application.
package db

import (
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/url"
	"strings"
)

// MySQLProvider implements the db.Provider interface for MySQL databases.
type MySQLProvider struct {
	cfg    config.Manager // Configuration manager
	sqlDB  *gorm.DB       // GORM database instance
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
	dbConfig := p.cfg.Config().Core.DB
	
	// Build query parameters
	query := url.Values{}
	query.Set("parseTime", "True")
	query.Set("loc", "Local")

	if dbConfig.TLSEnabled {
		logger.Debug("TLS enabled")
		if dbConfig.TLSSkipVerify {
			logger.Debug("Skipping TLS verification")
			query.Set("tls", "skip-verify")
		} else {
			query.Set("tls", "true")
		}
	}

	if dbConfig.Charset != "" {
		query.Set("charset", dbConfig.Charset)
		logger.Debug("Setting charset", zap.String("charset", dbConfig.Charset))
	} else {
		logger.Debug("Charset is empty, skipping parameter")
	}

	// Construct the DSN using url.URL
	u := &url.URL{
		Scheme:   "tcp",
		User:     url.UserPassword(dbConfig.Username, dbConfig.Password),
		Host:     fmt.Sprintf("%s:%d", dbConfig.Host, dbConfig.Port),
		Path:     dbConfig.Name,
		RawQuery: query.Encode(),
	}

	// Decode the user info portion since MySQL doesn't expect it to be URL encoded
	userStr, err := url.QueryUnescape(u.User.String())
	if err != nil {
		return nil, fmt.Errorf("failed to unescape user string: %v", err)
	}

	// Format as MySQL DSN with the decoded user string
	dsn := fmt.Sprintf("%s@%s(%s)/%s?%s",
		userStr,
		u.Scheme,
		u.Host,
		strings.TrimPrefix(u.Path, "/"),
		u.RawQuery,
	)

	logger.Debug("Connecting to MySQL", zap.String("dsn", dsn))

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger(logger.Logger, logger.Level()),
	})
	
	if err != nil {
		return nil, err
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
