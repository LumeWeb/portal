package models

import (
	"time"

	"gorm.io/gorm"
)

func init() {
	registerModel(&SCPriceHistory{})
}

type SCPriceHistory struct {
	gorm.Model
	CreatedAt time.Time `gorm:"index:idx_rate"`
	Rate      float64   `gorm:"index:idx_rate"`
}
