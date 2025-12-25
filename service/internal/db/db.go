package db

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"gorm.io/gorm"
)

// HandleDBError standardizes database error handling across services
func HandleDBError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Optionally: detect duplicate key and map to a friendlier error key
	// if errors.Is(err, gorm.ErrDuplicatedKey) { return core.NewAccountError(core.ErrKeyDuplicate, err) }

	return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
}
