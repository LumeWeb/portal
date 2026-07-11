package models

import (
	"gorm.io/gorm"
)

func init() {
	registerModel(&ObjectSyncCursor{})
}

// ObjectSyncCursor stores the cursor position for the sealed-data refresh
// loop. There is always at most one row (singleton, keyed by the constant
// default key). The cursor tracks progress through the indexer's object
// event feed so that the loop can resume after restart.
type ObjectSyncCursor struct {
	gorm.Model
	Cursor string // JSON-encoded slabs.Cursor
}

func (ObjectSyncCursor) TableName() string { return "object_sync_cursor" }
