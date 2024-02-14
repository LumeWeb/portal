package models

import (
	"errors"
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

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	if len(u.FirstName) == 0 {
		return errors.New("first name is empty")
	}

	if len(u.LastName) == 0 {
		return errors.New("last name is empty")
	}
	return nil
}
