package models

import (
	"gorm.io/gorm"
)

func init() {
	registerModel(&AccessRule{})
}

type AccessRule struct {
	gorm.Model
	Ptype string
	V0    string
	V1    string
	V2    string
	V3    string
	V4    string
	V5    string
}
