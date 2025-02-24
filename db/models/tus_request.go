package models

import (
	"errors"
	"go.lumeweb.com/portal/db/models/data_models"
	"gorm.io/gorm"
)

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

func (t *TUSRequest) TableName() string {
	return "tus_requests"
}

func (t *TUSRequest) Validate() error {
	if t.TUSUploadID == "" {
		return errors.New("tus upload ID is required")
	}
	return nil
}

func (t *TUSRequest) NewInstance() data_models.RequestDataModel {
	return &TUSRequest{}
}

func (t *TUSRequest) SetRequestID(id uint) {
	t.RequestID = id
}

func (t *TUSRequest) GetRequestID() uint {
	return t.RequestID
}
