package models

import "gorm.io/gorm"

type Upload struct {
	gorm.Model
	UserID       uint
	Hash         string
	ProtocolType string
	User         User
	UploaderIP   string
}
