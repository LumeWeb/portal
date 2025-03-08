package models

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
	mh "github.com/multiformats/go-multihash"
)

type HashMapping struct {
	gorm.Model
	SourceHash mh.Multihash
	TargetHash mh.Multihash
	Protocol   string
	Metadata   datatypes.JSON
}

func init() {
	registerModel(&HashMapping{})
}
