package models

import (
	"gorm.io/gorm"
)

type CronJobLogType string

const (
	CronJobLogTypeFailure CronJobLogType = "failure"
)

func init() {
	registerModel(&CronJob{})
}

type CronJobLog struct {
	gorm.Model
	CronJobID uint
	CronJob   CronJob
	Type      CronJobLogType
	Message   string
}
