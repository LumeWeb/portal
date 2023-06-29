package model

import (
	"gorm.io/gorm"
)

type Tus struct {
	gorm.Model
	ID        uint64 `gorm:"primaryKey"`
	UploadID  string
	Hash      string
	Info      string
	AccountID uint
	Account   Account
}
