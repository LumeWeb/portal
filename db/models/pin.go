package models

import "gorm.io/gorm"

type Pin struct {
	gorm.Model
	UploadID uint
	Upload   Upload
	UserID   uint
	User     User
}
