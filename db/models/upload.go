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
	Hash       mh.Multihash
	CIDType    uint64 `gorm:"column:cid_type"` // Keep column name mapping
	MimeType   string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
	Metadata   datatypes.JSON
	DeletedAt  gorm.DeletedAt
}
