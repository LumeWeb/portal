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

	// Operation types
	RequestOperationTusUpload = "tus.upload"
)

type Request struct {
	gorm.Model
	Operation         string
	Protocol          string
	Status            RequestStatusType
	StatusMessage     string
	System            bool
	UserID            uint
	User              User
	SourceIP          string
	HashType          uint64
	Hash              mh.Multihash
	CIDType           uint64 `gorm:"column:cid_type"`
	UploadHash        mh.Multihash
	UploadHashCIDType uint64 `gorm:"column:upload_hash_cid_type"`
	Size              uint64
	MimeType          string
	Metadata          datatypes.JSON
}
