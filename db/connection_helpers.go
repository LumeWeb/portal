// In db/connection_helpers.go (new file)

package db

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/config"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// GetDSN generates the DSN string for GORM based on the configuration.
func GetDSN(cfg config.Manager) (string, error) {
	dbConfig := cfg.Config().Core.DB
	if dbConfig.Type == "" {
		return "", errors.New("database type is required")
	}

	switch strings.ToLower(dbConfig.Type) {
	case "sqlite":
		if dbConfig.File == "" {
			return "", errors.New("sqlite database requires a file path")
		}
		filePath := GetSQLiteDBFile(cfg)
		if !strings.HasPrefix(filePath, "file:") {
			filePath = "file:" + filePath
		}
		return filePath, nil
	case "sqlitememory":
		return "file:memory?mode=memory&cache=shared", nil

	case "mysql":
		if dbConfig.Host == "" {
			return "", errors.New("mysql database requires a host")
		}
		if dbConfig.Name == "" {
			return "", errors.New("mysql database requires a database name")
		}

		// Build query parameters for MySQL DSN
		query := url.Values{}
		query.Set("parseTime", "True")
		query.Set("loc", "Local")

		if dbConfig.TLSEnabled {
			if dbConfig.TLSSkipVerify {
				query.Set("tls", "skip-verify")
			} else {
				query.Set("tls", "true")
			}
		}

		if dbConfig.Charset != "" {
			query.Set("charset", dbConfig.Charset)
		}

		// Format as standard MySQL DSN
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
			dbConfig.Username,
			dbConfig.Password,
			dbConfig.Host,
			dbConfig.Port,
			dbConfig.Name,
			query.Encode(),
		)
		return dsn, nil

	default:
		return "", fmt.Errorf("unsupported database type: %s", dbConfig.Type)
	}
}

// GetDbMateUrl generates a database connection URL for dbmate based on the configuration.
// It uses GetDSN() and converts the DSN to dbmate's expected URL format.
func GetDbMateUrl(cfg config.Manager) (*url.URL, error) {
	dsn, err := GetDSN(cfg)
	if err != nil {
		return nil, err
	}

	dbType := cfg.Config().Core.DB.Type

	switch strings.ToLower(dbType) {
	case "sqlite":
		// For SQLite file, dbmate expects sqlite:// prefix
		if strings.HasPrefix(dsn, "file:") {
			dsn = strings.TrimPrefix(dsn, "file:")
		}
		return url.Parse("sqlite://" + dsn)
	case "sqlitememory":
		// Use our custom driver for in-memory SQLite
		return url.Parse("sqlitememory://memory")

	case "mysql":
		// Parse MySQL DSN using driver's parser
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MySQL DSN: %w", err)
		}

		// Construct URL from parsed config
		u := &url.URL{
			Scheme: "mysql",
			User:   url.UserPassword(cfg.User, cfg.Passwd),
			Host:   cfg.Addr,
			Path:   "/" + cfg.DBName,
		}

		// Convert DSN parameters to URL query
		query := url.Values{}
		if cfg.Params != nil {
			for k, v := range cfg.Params {
				query.Set(k, v)
			}
		}
		if cfg.TLS != nil {
			if cfg.TLS.ServerName != "" {
				query.Set("tls", cfg.TLS.ServerName)
			} else {
				query.Set("tls", "skip-verify")
			}
		}
		if cfg.Timeout > 0 {
			query.Set("timeout", cfg.Timeout.String())
		}
		if cfg.ReadTimeout > 0 {
			query.Set("readTimeout", cfg.ReadTimeout.String())
		}
		if cfg.WriteTimeout > 0 {
			query.Set("writeTimeout", cfg.WriteTimeout.String())
		}
		u.RawQuery = query.Encode()

		return u, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}
