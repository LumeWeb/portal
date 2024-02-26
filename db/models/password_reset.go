package models

import (
	"time"

	"gorm.io/gorm"
)

type PasswordReset struct {
	gorm.Model

	UserID    uint
	User      User
	Token     string
	ExpiresAt time.Time
}
