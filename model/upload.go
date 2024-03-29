package model

import (
	"gorm.io/gorm"
)

type Upload struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	AccountID uint `gorm:"index"`
	Account   Account
	Hash      string `gorm:"uniqueIndex"`
}
