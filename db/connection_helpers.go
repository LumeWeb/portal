// In db/connection_helpers.go (new file)

package db

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/config"
	"net/url"
	"strings"
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
