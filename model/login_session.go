package model

import (
	"gorm.io/gorm"
	"time"
)

type LoginSession struct {
	gorm.Model
	ID         uint    `gorm:"primaryKey"`
	Token      string  `gorm:"uniqueIndex"`
	Account    Account `gorm:"foreignKey:AccountID"`
	Expiration time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (s *LoginSession) BeforeCreate(tx *gorm.DB) (err error) {
	s.Expiration = time.Now().Add(time.Hour * 24)
	return
}
