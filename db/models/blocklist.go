package models

import (
	"time"

	"gorm.io/gorm"
)

func init() {
	registerModel(&Blocklist{})
}

type Blocklist struct {
	gorm.Model
	IP        string
	Reason    string
	BlockedAt time.Time
}

func (Blocklist) TableName() string {
	return "blocklist"
}
