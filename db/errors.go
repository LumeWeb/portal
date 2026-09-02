package db

import (
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// IsDuplicateKeyError reports whether err is a unique-constraint violation on
// either MySQL or SQLite.
func IsDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr != nil && mysqlErr.Number == 1062 {
		return true
	}

	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
		return true
	}

	return false
}

// IsConstraintViolationError reports whether err is a non-unique constraint
// violation (foreign key, check, out-of-range, or null) on MySQL or GORM.
func IsConstraintViolationError(err error) bool {
	if errors.Is(err, gorm.ErrForeignKeyViolated) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
		return true
	}

	// MySQL constraint violation error codes
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr != nil {
		switch mysqlErr.Number {
		case 1452: // Cannot add or update a child row: a foreign key constraint fails
			return true
		case 1451: // Cannot delete or update a parent row: a foreign key constraint fails
			return true
		case 1264: // Out of range value
			return true
		case 1048: // Column cannot be null
			return true
		}
	}

	return false
}
