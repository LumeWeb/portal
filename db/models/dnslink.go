package models

import "gorm.io/gorm"

func init() {
	registerModel(&DNSLink{})
}

type DNSLink struct {
	gorm.Model
	UserID   uint `gorm:"uniqueIndex:idx_user_id_upload"`
	User     User
	UploadID uint `gorm:"uniqueIndex:idx_user_id_upload"`
	Upload   Upload
}
