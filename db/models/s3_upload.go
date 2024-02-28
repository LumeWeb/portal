package models

import "gorm.io/gorm"

type S3Upload struct {
	gorm.Model
	UploadID string `gorm:"unique;not null"`
	Bucket   string `gorm:"not null"`
	Key      string `gorm:"not null"`
}
