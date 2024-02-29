package models

import "gorm.io/gorm"

func init() {
	registerModel(&APIKey{})
}

type APIKey struct {
	gorm.Model
	UserID uint
	Key    string
	User   User
}
