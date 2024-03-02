package models

import "gorm.io/gorm"

func init() {
	registerModel(&SiaUpload{})
}

type SiaUpload struct {
	gorm.Model
	UploadID string `gorm:"unique;not null"`
	Bucket   string `gorm:"not null;index:idx_bucket_key"`
	Key      string `gorm:"not null;index:idx_bucket_key"`
}
