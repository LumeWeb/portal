package models

import "gorm.io/gorm"

func init() {
	registerModel(&S3Upload{})
}

type S3Upload struct {
	gorm.Model
	UploadID string `gorm:"unique;not null"`
	Bucket   string `gorm:"not null;index:idx_s3_bucket_key"`
	Key      string `gorm:"not null;index:idx_s3_bucket_key"`
}
