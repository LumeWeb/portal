package models

import (
	"go.lumeweb.com/portal/db/types"
	"gorm.io/gorm"
	"time"
)

type CronJobState string

const (
	CronJobStateQueued    CronJobState = "queued"
	CronJobStateRunning   CronJobState = "running"
	CronJobStateCompleted CronJobState = "completed"
	CronJobStateFailed    CronJobState = "failed"
)

var cronJobStateNames = map[CronJobState]string{
	CronJobStateQueued:    "Queued",
	CronJobStateRunning:   "Running",
	CronJobStateCompleted: "Completed",
	CronJobStateFailed:    "Failed",
}

// DisplayName returns the human-readable name for a cron job state
func (s CronJobState) DisplayName() string {
	if name, ok := cronJobStateNames[s]; ok {
		return name
	}
	return string(s)
}

func init() {
	registerModel(&CronJob{})
}

type CronJob struct {
	gorm.Model
	UUID          types.BinaryUUID
	Origin        string
	SourceID      string
	JobType       string
	Args          string
	SchedDef      string
	ScheduleType  string
	State         CronJobState
	LastRun       *time.Time
	LastHeartbeat *time.Time
	Failures      uint
	Version       int64
}

func (c *CronJob) TableName() string {
	return "cron_jobs"
}
