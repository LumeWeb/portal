package models

import "gorm.io/gorm"

type Upload struct {
	gorm.Model
	UserID     uint
	Hash       string
	Protocol   string
	User       User
	UploaderIP string
	Size       uint64
}
