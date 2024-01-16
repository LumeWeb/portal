package models

import (
	"gorm.io/gorm"
	"time"
)

type Blocklist struct {
	gorm.Model
	IP        string
	Reason    string
	BlockedAt time.Time
}
