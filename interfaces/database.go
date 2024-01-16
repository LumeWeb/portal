package interfaces

import "gorm.io/gorm"

type Database interface {
	Init(p Portal) error
	Start() error
	Get() *gorm.DB
}
