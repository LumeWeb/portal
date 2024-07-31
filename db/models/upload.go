package models

import (
	mh "github.com/multiformats/go-multihash"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func init() {
	registerModel(&Upload{})
}

type Upload struct {
	gorm.Model
	UserID     uint
	HashType   uint64
	Hash       mh.Multihash `gorm:"type:binary(64);uniqueIndex:idx_upload_hash_deleted_at"`
	MimeType   string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
	Metadata   datatypes.JSON
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_upload_hash_deleted_at"`
}
