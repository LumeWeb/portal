package models

import (
	mh "github.com/multiformats/go-multihash"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type (
	RequestStatusType    string
	RequestOperationType string
)

const (
	RequestStatusPending    RequestStatusType = "pending"
	RequestStatusProcessing RequestStatusType = "processing"
	RequestStatusCompleted  RequestStatusType = "completed"

	RequestStatusFailed       RequestStatusType    = "failed"
	RequestOperationUpload    RequestOperationType = "post_upload"
	RequestOperationTusUpload RequestOperationType = "tus_upload"
	RequestOperationPin       RequestOperationType = "pin"
)

type Request struct {
	gorm.Model
	Operation  RequestOperationType `gorm:"index:idx_request_operation_system"`
	Protocol   string
	Status     RequestStatusType
	System     bool `gorm:"default:false;index:idx_request_operation_system"`
	UserID     uint
	User       User
	SourceIP   string
	HashType   uint64
	Hash       mh.Multihash `gorm:"type:varbinary(64);index"`
	CIDType    uint64       `gorm:"null;column:cid_type"`
	UploadHash mh.Multihash `gorm:"type:varbinary(64);index"`
	Size       uint64
	MimeType   string
	Metadata   datatypes.JSON
}
