package models

import (
	mh "github.com/multiformats/go-multihash"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func init() {
	registerModel(&ScanResult{})
}

type ScanResult struct {
	gorm.Model
	Hash      mh.Multihash
	ScannerID string
	Passed    bool
	Reason    string
	Metadata  datatypes.JSON
}
