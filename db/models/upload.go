package models

import "gorm.io/gorm"

type Upload struct {
	gorm.Model
	UserID       uint
	CID          string
	ProtocolType string
	User         User
	UploaderIP   string
}
