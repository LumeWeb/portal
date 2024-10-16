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
	Hash       mh.Multihash `gorm:"type:varbinary(64);uniqueIndex:idx_upload_hash_deleted_at"`
	CIDType    uint64       `gorm:"column:cid_type"`
	MimeType   string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
	Metadata   datatypes.JSON
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_upload_hash_deleted_at"`
}
