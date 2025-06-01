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
	UUID          types.BinaryUUID `gorm:"uniqueIndex"`
	Function      string
	Args          string
	LastRun       *time.Time
	Failures      uint64
	State         CronJobState
	LastHeartbeat *time.Time
	Version       uint64
}

func (t *CronJob) BeforeCreate(_ *gorm.DB) error {
	id, err := uuid.NewRandom()
	t.UUID = types.ParseUUID(id)
	return err
}
