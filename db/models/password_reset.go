package models

import (
	"time"

	"gorm.io/gorm"
)

func init() {
	registerModel(&PasswordReset{})
}

type PasswordReset struct {
	gorm.Model

	UserID    uint
	User      User
	Token     string
	ExpiresAt time.Time
}
