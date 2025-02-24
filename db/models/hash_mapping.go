package models

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
	mh "github.com/multiformats/go-multihash"
)

type HashMapping struct {
	gorm.Model
	SourceHash mh.Multihash `gorm:"type:varbinary(64);index:idx_hash_mapping_source"`
	TargetHash mh.Multihash `gorm:"type:varbinary(64);index:idx_hash_mapping_target"`
	Protocol   string       `gorm:"type:varchar(255);index:idx_hash_mapping_protocol"`
	Metadata   datatypes.JSON
}

func init() {
	registerModel(&HashMapping{})
}
