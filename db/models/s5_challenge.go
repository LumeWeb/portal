package models

import "gorm.io/gorm"

func init() {
	registerModel(&S5Challenge{})

}

type S5Challenge struct {
	gorm.Model
	Challenge string
	Pubkey    string
	Type      string
}
