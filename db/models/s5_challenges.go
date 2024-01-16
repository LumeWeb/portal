package models

import "gorm.io/gorm"

type S5Challenge struct {
	gorm.Model
	Challenge string
	Pubkey    string
	Type      string
}
