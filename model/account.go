package model

import (
	"gorm.io/gorm"
	"time"
)

type Account struct {
	gorm.Model
	ID          uint   `gorm:"primaryKey" gorm:"AUTO_INCREMENT"`
	Email       string `gorm:"uniqueIndex"`
	Password    *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LoginTokens []LoginSession
	Keys        []Key
}
