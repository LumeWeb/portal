package models

import "gorm.io/gorm"

func init() {
	registerModel(&Pin{})
}

type Pin struct {
	gorm.Model
	UploadID uint
	Upload   Upload
	UserID   uint
	User     User
}
