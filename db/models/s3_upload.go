package models

import "gorm.io/gorm"

func init() {
	registerModel(&S3Upload{})
}

type S3Upload struct {
	gorm.Model
	UploadID string `gorm:"unique"`
	Bucket   string
	Key      string
}
