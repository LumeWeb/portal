package model

import (
	"gorm.io/gorm"
	"time"
)

type KeyChallenge struct {
	gorm.Model
	ID         uint `gorm:"primaryKey"`
	AccountID  uint
	Account    Account
	Challenge  string `gorm:"not null"`
	Expiration time.Time
}
