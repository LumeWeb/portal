package model

import "gorm.io/gorm"

type Pin struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	AccountID uint `gorm:"uniqueIndex:idx_account_upload"`
	UploadID  uint `gorm:"uniqueIndex:idx_account_upload"`
	Account   Account
	Upload    Upload
}
