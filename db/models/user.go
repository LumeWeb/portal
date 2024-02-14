package models

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	gorm.Model
	FirstName    string
	LastName     string
	Email        string `gorm:"unique"`
	PasswordHash string
	Role         string
	PublicKeys   []PublicKey
	APIKeys      []APIKey
	Uploads      []Upload
	LastLogin    *time.Time
	LastLoginIP  string
}
