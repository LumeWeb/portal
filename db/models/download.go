package models

import (
	"time"

	"gorm.io/gorm"
)

func init() {
	registerModel(&Download{})
}

type Download struct {
	gorm.Model
	UserID       uint
	User         User
	UploadID     uint
	Upload       Upload
	DownloadedAt time.Time
	IP           string
}
