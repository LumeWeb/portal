package model

import (
	"gorm.io/gorm"
	"time"
)

type Key struct {
	gorm.Model
	ID         uint    `gorm:"primaryKey"`
	Account    Account `gorm:"references:ID"`
	PublicKey  string
	PrivateKey string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
