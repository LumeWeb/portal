package models

import (
	"gorm.io/gorm"
)

func init() {
	registerModel(&AccountDeletion{})
}

type AccountDeletion struct {
	gorm.Model
	IP     string
	UserID uint
	User   User
}
