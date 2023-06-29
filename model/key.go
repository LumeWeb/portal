package model

import (
	"gorm.io/gorm"
	"time"
)

type Key struct {
	gorm.Model
	ID        uint `gorm:"primaryKey" gorm:"AUTO_INCREMENT"`
	AccountID uint
	Account   Account
	Pubkey    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
