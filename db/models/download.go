package models

import (
	"gorm.io/gorm"
	"time"
)

type Download struct {
	gorm.Model
	UserID       uint
	User         User
	UploadID     uint
	Upload       Upload
	DownloadedAt time.Time
	IP           string
}
