package models

import (
	"git.lumeweb.com/LumeWeb/portal/db/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func init() {
	registerModel(&CronJob{})
}

type CronJob struct {
	gorm.Model
	UUID     types.BinaryUUID
	Tags     []string `gorm:"serializer:json;type:text;"`
	Function string   `gorm:"type:varchar(255);"`
	Args     string   `gorm:"type:text;"`
}

func (t *CronJob) BeforeCreate(_ *gorm.DB) error {
	id, err := uuid.NewRandom()
	t.UUID = types.BinaryUUID(id)
	return err
}
