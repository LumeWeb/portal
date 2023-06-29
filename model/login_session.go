package model

import (
	"gorm.io/gorm"
	"time"
)

type LoginSession struct {
	gorm.Model
	ID         uint `gorm:"primaryKey" gorm:"AUTO_INCREMENT"`
	AccountID  uint
	Account    Account
	Token      string `gorm:"uniqueIndex"`
	Expiration time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (s *LoginSession) BeforeCreate(tx *gorm.DB) (err error) {
	s.Expiration = time.Now().Add(time.Hour * 24)
	return
}
