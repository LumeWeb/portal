package models

import (
	"github.com/google/uuid"
	"go.lumeweb.com/portal/db/types"
	"gorm.io/gorm"
	"time"
)

func init() {
	registerModel(&CronJob{})
}

type CronJob struct {
	gorm.Model
	UUID     types.BinaryUUID
	Function string `gorm:"type:varchar(255);"`
	Args     string `gorm:"type:longtext;"`
	LastRun  *time.Time
	Failures uint
}

func (t *CronJob) BeforeCreate(_ *gorm.DB) error {
	id, err := uuid.NewRandom()
	t.UUID = types.BinaryUUID(id)
	return err
}
