package models

import "gorm.io/gorm"

type TusUpload struct {
	gorm.Model
	Hash       string `gorm:"uniqueIndex:idx_hash_deleted"`
	MimeType   string
	UploadID   string `gorm:"uniqueIndex"`
	UploaderID uint
	UploaderIP string
	Uploader   User `gorm:"foreignKey:UploaderID"`
	Protocol   string
	Completed  bool
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_hash_deleted"`
}
