package models

import "gorm.io/gorm"

func init() {
	registerModel(&Upload{})
}

type Upload struct {
	gorm.Model
	UserID     uint
	Hash       []byte `gorm:"type:binary(32);unique_index"`
	MimeType   string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
}
