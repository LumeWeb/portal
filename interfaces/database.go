package interfaces

import "gorm.io/gorm"

type Database interface {
	Get() *gorm.DB
	Service
}
