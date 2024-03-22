package models

import "gorm.io/gorm"

type ImportStatus string

const (
	ImportStatusQueued     ImportStatus = "queued"
	ImportStatusProcessing ImportStatus = "processing"
	ImportStatusCompleted  ImportStatus = "completed"
)

func init() {
	registerModel(&Import{})
}

type Import struct {
	gorm.Model
	UserID     uint
	Hash       []byte `gorm:"type:binary(32);"`
	Protocol   string
	User       User
	ImporterIP string
	Status     ImportStatus
	Progress   float64
}
