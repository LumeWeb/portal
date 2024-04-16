package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func init() {
	registerModel(&CronJob{})
}

type CronJob struct {
	gorm.Model
	UUID     uuid.UUID `gorm:"type:varchar(16);"`
	Tags     []string  `gorm:"serializer:json;type:text;"`
	Function string    `gorm:"type:varchar(255);"`
	Args     string    `gorm:"type:text;"`
}
