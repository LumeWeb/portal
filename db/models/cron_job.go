package models

import (
	"github.com/google/uuid"
	"go.lumeweb.com/portal/db/types"
	"gorm.io/gorm"
	"time"
)

type CronJobState string

const (
	CronJobStateQueued     CronJobState = "queued"
	CronJobStateProcessing CronJobState = "processing"
	CronJobStateCompleted  CronJobState = "completed"
	CronJobStateFailed     CronJobState = "failed"
)

func init() {
	registerModel(&CronJob{})
}

type CronJob struct {
	gorm.Model
	UUID          types.BinaryUUID `gorm:"type:binary(16);uniqueIndex"`
	Function      string           `gorm:"type:varchar(255);"`
	Args          string           `gorm:"type:longtext;"`
	LastRun       *time.Time
	Failures      uint64
	State         CronJobState `gorm:"type:varchar(20);default:'queued'"`
	LastHeartbeat *time.Time
	Version       uint64 `gorm:"default:0"`
}

func (t *CronJob) BeforeCreate(_ *gorm.DB) error {
	id, err := uuid.NewRandom()
	t.UUID = types.BinaryUUID(id)
	return err
}
