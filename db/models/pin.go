package models

import "gorm.io/gorm"

type Pin struct {
	gorm.Model
	UploadID       uint
	Upload         Upload
	PinnedByUserID uint
	User           User
}
