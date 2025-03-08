package models

import "gorm.io/gorm"

func init() {
	registerModel(&SiaUpload{})
}

type SiaUpload struct {
	gorm.Model
	UploadID string `gorm:"unique"`
	Bucket   string
	Key      string
}
