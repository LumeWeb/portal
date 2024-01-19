package models

import "gorm.io/gorm"

type TusUpload struct {
	gorm.Model
	Hash       string `gorm:"uniqueIndex"`
	UploadID   string `gorm:"uniqueIndex"`
	UploaderID uint
	UploaderIP string
	Uploader   User `gorm:"foreignKey:UploaderID"`
	Protocol   string
}
