package models

import (
	"time"

	"gorm.io/gorm"
)

type EmailVerification struct {
	gorm.Model

	UserID    uint
	User      User
	NewEmail  string
	Token     string
	ExpiresAt time.Time
}
