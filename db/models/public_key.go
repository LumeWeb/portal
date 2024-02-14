package models

import "gorm.io/gorm"

type PublicKey struct {
	gorm.Model
	UserID uint
	Key    string `gorm:"unique;not null"`
	User   User
}
