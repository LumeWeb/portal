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
	UUID     uuid.UUID
	Tags     []string
	Function string
	Args     string
}
