package interfaces

import "gorm.io/gorm"

type Database interface {
	Init() error
	Start() error
	Get() *gorm.DB
}
