package models

import (
	"time"

	"github.com/shopspring/decimal"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var _ schema.Tabler = (*SCPriceHistory)(nil)

func init() {
	registerModel(&SCPriceHistory{})
}

type SCPriceHistory struct {
	gorm.Model
	CreatedAt time.Time       `gorm:"index:idx_rate"`
	Rate      decimal.Decimal `gorm:"type:DECIMAL(10,20);index:idx_rate"`
}

func (SCPriceHistory) TableName() string {
	return "sc_price_history"
}
