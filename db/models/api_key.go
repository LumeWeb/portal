package models

import "gorm.io/gorm"

type APIKey struct {
	gorm.Model
	UserID uint
	Key    string
	User   User
}
