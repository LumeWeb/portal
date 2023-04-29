package model

import (
	"gorm.io/gorm"
	"time"
)

type KeyChallenge struct {
	gorm.Model
	ID         uint    `gorm:"primaryKey"`
	Account    Account `gorm:"foreignKey:AccountID"`
	Challenge  string  `gorm:"not null"`
	Expiration time.Time
}
