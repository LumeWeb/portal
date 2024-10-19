package models

import (
	"gorm.io/gorm"
)

func init() {
	registerModel(&AccessRule{})
}

type AccessRule struct {
	gorm.Model
	Ptype string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V0    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V1    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V2    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V3    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V4    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
	V5    string `gorm:"size:512;index:idx_access_rule,unique,length:100"`
}
