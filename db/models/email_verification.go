package models

import (
	"time"

	"gorm.io/gorm"
)

func init() {
	registerModel(&EmailVerification{})
}

type EmailVerification struct {
	gorm.Model

	UserID    uint
	User      User
	NewEmail  string
	Token     string
	ExpiresAt time.Time
}
