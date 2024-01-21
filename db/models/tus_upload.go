package models

import "gorm.io/gorm"

type TusUpload struct {
	gorm.Model
	Hash       string `gorm:"uniqueIndex:idx_hash_deleted"`
	UploadID   string `gorm:"uniqueIndex"`
	UploaderID uint
	UploaderIP string
	Uploader   User `gorm:"foreignKey:UploaderID"`
	Protocol   string
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_hash_deleted"`
}
