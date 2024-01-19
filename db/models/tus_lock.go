package models

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var (
	ErrTusLockBusy = errors.New("lock is currently held by another process")
)

type TusLock struct {
	gorm.Model
	LockId           string `gorm:"uniqueIndex"`
	HolderPID        int    `gorm:"index"`
	AcquiredAt       time.Time
	ExpiresAt        time.Time
	ReleaseRequested bool `gorm:"default:false"`
}

func (t *TusLock) TryLock(db *gorm.DB, ctx context.Context) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var existingLock TusLock

		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("lock_id = ?", t.LockId).First(&existingLock).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Insert new lock record
				err := tx.WithContext(ctx).Create(t).Error
				if err != nil {
					return err
				}

				t = &existingLock
				return nil
			}

			return err
		}

		// Check if existing lock is expired
		if existingLock.ExpiresAt.Before(time.Now()) || existingLock.ReleaseRequested {

			err := tx.Model(&existingLock).Updates(t).Error
			if err != nil {
				return err
			}

			t = &existingLock

			return nil
		}

		// Lock is currently held by another process
		return ErrTusLockBusy
	})
}
func (t *TusLock) RequestRelease(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Update the ReleaseRequested flag in the database for the specific lock
		return tx.Model(t).Where("lock_id = ?", t.LockId).Update("release_requested", true).Error
	})
}
func (t *TusLock) Released(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Update the ReleaseRequested flag in the database for the specific lock
		return tx.Model(t).Where("lock_id = ?", t.LockId).Update("release_requested", false).Error
	})
}

func (t *TusLock) IsReleaseRequested(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Model(&TusLock{}).Where(&TusLock{LockId: t.LockId, ReleaseRequested: true}).Count(&count).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}

	return count > 0, nil
}
func (t *TusLock) Delete(db *gorm.DB) error {
	return db.Where("lock_id = ?", t.LockId).Delete(&TusLock{}).Error
}
