package model

import (
	"gorm.io/gorm"
)

type Tus struct {
	gorm.Model
	ID        uint `gorm:"primaryKey" gorm:"AUTO_INCREMENT"`
	UploadID  string
	Hash      string
	Info      string
	AccountID uint
	Account   Account
}
