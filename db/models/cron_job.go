package models

import (
	"github.com/google/uuid"
	"go.lumeweb.com/portal/db/types"
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
	Args     string   `gorm:"type:longtext;"`
}

func (t *CronJob) BeforeCreate(_ *gorm.DB) error {
	id, err := uuid.NewRandom()
	t.UUID = types.BinaryUUID(id)
	return err
}
