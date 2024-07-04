package models

import "gorm.io/gorm"

func init() {
	registerModel(&Upload{})
}

type Upload struct {
	gorm.Model
	UserID     uint
	HashType   uint
	Hash       []byte `gorm:"type:binary(64);uniqueIndex:idx_upload_hash_deleted_at"`
	MimeType   string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_upload_hash_deleted_at"`
}
