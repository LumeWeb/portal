package models

import "gorm.io/gorm"

func init() {
	registerModel(&PublicKey{})
}

type PublicKey struct {
	gorm.Model
	UserID uint
	Key    string `gorm:"unique;not null"`
	User   User
}
