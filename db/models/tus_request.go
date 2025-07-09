package models

import (
	"errors"
	"fmt"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/db/models/data_models"
	"gorm.io/gorm"
)

func init() {
	registerModel(&TUSRequest{})
}

var _ data_models.RequestDataModel = (*TUSRequest)(nil)

type TUSRequest struct {
	gorm.Model
	RequestID   uint `gorm:"request_id"`
	Request     Request
	TUSUploadID string       `gorm:"tus_upload_id"`
	UploadHash  mh.Multihash `gorm:"upload_hash"`
	Completed   bool         `gorm:"completed"`
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

func (t *TUSRequest) SetRequest(req any) {
	if _mreq, ok := req.(*Request); ok {
		t.Request = *_mreq
		return
	}
	if _mreq, ok := req.(Request); ok {
		t.Request = _mreq
		return
	}
	panic(fmt.Sprintf("invalid request type %T, expected *Request or Request", req))
}
