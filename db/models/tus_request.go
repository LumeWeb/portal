package models

import "gorm.io/gorm"

func init() {
	registerModel(&TUSRequest{})
}

type TUSRequest struct {
	gorm.Model
	RequestID   uint `gorm:"uniqueIndex:idx_tus_requests_request_id"`
	Request     Request
	TUSUploadID string `gorm:"type:varchar(500);uniqueIndex"`
	Completed   bool
}
