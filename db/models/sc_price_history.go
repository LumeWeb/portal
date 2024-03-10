package models

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var _ schema.Tabler = (*SCPriceHistory)(nil)

func init() {
	registerModel(&SCPriceHistory{})
}

type SCPriceHistory struct {
	gorm.Model
	CreatedAt time.Time `gorm:"index:idx_rate"`
	Rate      float64   `gorm:"index:idx_rate"`
}

func (SCPriceHistory) TableName() string {
	return "sc_price_history"
}
