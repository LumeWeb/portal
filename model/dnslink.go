package model

import (
	"gorm.io/gorm"
)

type Dnslink struct {
	gorm.Model
	ID     uint   `gorm:"primaryKey" gorm:"AUTO_INCREMENT"`
	Domain string `gorm:"uniqueIndex"`
}
