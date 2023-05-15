package model

import (
	"gorm.io/gorm"
)

type Tus struct {
	gorm.Model
	UploadID string `gorm:"primaryKey"`
	Path     string
	Hash     string
}
