package models

import (
	mh "github.com/multiformats/go-multihash"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type (
	RequestStatusType string
)

const (
	RequestStatusPending    RequestStatusType = "pending"
	RequestStatusProcessing RequestStatusType = "processing"
	RequestStatusCompleted  RequestStatusType = "completed"
	RequestStatusFailed     RequestStatusType = "failed"
	RequestStatusDuplicate  RequestStatusType = "duplicate"
)

type Request struct {
	gorm.Model
	Operation     string
	Protocol      string
	Status        RequestStatusType
	StatusMessage string
	UserID        *uint
	User          User
	SourceIP      string
	Hash          mh.Multihash
	CIDType       uint64 `gorm:"column:cid_type"`
	Metadata      datatypes.JSON
}
