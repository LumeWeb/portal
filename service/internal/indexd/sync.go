package indexd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/indexd/slabs"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// syncInterval is how often the sealed-data refresh loop runs.
const syncInterval = 24 * time.Hour

// syncBatchSize is the number of object events fetched per API call.
const syncBatchSize = 100

// SyncLoopCfg holds the dependencies for the sealed-data refresh loop.
type SyncLoopCfg struct {
	Component core.Component
	Logger    *core.Logger
	SDK       SDK
}

// SealedDataSyncLoop periodically fetches object events from the indexer and
// refreshes the sealed data stored in renter_objects rows. The indexer may
// re-balance slabs across hosts over time; the sealed data we store locally
// contains slab locations and encryption keys, so it must be refreshed to
// keep downloads working.
//
// This mirrors s3d's syncMetadataLoop pattern: fetch events via cursor
// pagination, re-seal each object (which picks up updated slab info), and
// update the local DB row. Deleted objects in the feed are skipped — the
// portal manages its own deletion lifecycle.
func SealedDataSyncLoop(ctx context.Context, cfg SyncLoopCfg, done chan struct{}) {
	defer close(done)

	// Sync once on startup.
	if err := syncSealedData(ctx, cfg); err != nil {
		cfg.Logger.Error("sealed-data sync failed", zap.Error(err))
	}

	t := time.NewTicker(syncInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := syncSealedData(ctx, cfg); err != nil {
				cfg.Logger.Error("sealed-data sync failed", zap.Error(err))
			}
		}
	}
}

// syncSealedData fetches object events from the indexer since the last sync
// and updates the sealed_data column in renter_objects for any objects whose
// sealed data has changed.
func syncSealedData(ctx context.Context, cfg SyncLoopCfg) error {
	cursor, err := getSyncCursor(cfg.Component)
	if err != nil {
		return fmt.Errorf("failed to get sync cursor: %w", err)
	}

	var synced int
	for ctx.Err() == nil {
		events, err := cfg.SDK.ObjectEvents(ctx, cursor, syncBatchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch object events: %w", err)
		}
		if len(events) == 0 {
			break
		}

		for _, ev := range events {
			if ev.Deleted {
				// The portal manages its own deletion lifecycle. An object
				// deleted from the indexer may still have a local row if the
				// deletion was initiated by the portal. Skip — the local
				// deletion path handles cleanup.
				continue
			}
			if ev.Object == nil {
				continue
			}

			// Re-seal the object to pick up any updated slab info from the
			// indexer (e.g. re-balanced sectors, changed host set).
			sealed := cfg.SDK.SealObject(*ev.Object)
			sealedJSON, err := json.Marshal(sealed)
			if err != nil {
				cfg.Logger.Error("failed to marshal sealed object during sync",
					zap.Stringer("objectID", &ev.Key),
					zap.Error(err),
				)
				continue
			}

			siaObjectID := ev.Object.ID().String()
			var rawID [32]byte
			n, err := hex.Decode(rawID[:], []byte(siaObjectID))
			if err != nil || n != 32 {
				cfg.Logger.Error("failed to parse sia_object_id during sync",
					zap.String("siaObjectID", siaObjectID),
					zap.Error(err),
				)
				continue
			}

			// Update the sealed_data for the matching renter_objects row.
			// Only update rows in "uploaded" state — staged/packing objects
			// haven't been sealed yet.
			var rowsAffected int64
			if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
				result := db.WithContext(ctx).
					Model(&models.RenterObject{}).
					Where("sia_object_id = ? AND status = ?", siaObjectID, models.RenterObjectStatusUploaded).
					Update("sealed_data", datatypes.JSON(sealedJSON))
				rowsAffected = result.RowsAffected
				return result
			}); err != nil {
				cfg.Logger.Error("failed to update sealed_data during sync",
					zap.String("siaObjectID", siaObjectID),
					zap.Error(err),
				)
				continue
			}

			if rowsAffected > 0 {
				synced++
			}
		}

		// Advance the cursor to the last event.
		last := events[len(events)-1]
		cursor = slabs.Cursor{After: last.UpdatedAt, Key: last.Key}
		if err := setSyncCursor(cfg.Component, cursor); err != nil {
			return fmt.Errorf("failed to update sync cursor: %w", err)
		}
	}

	if synced > 0 {
		cfg.Logger.Info("synced sealed data from indexer", zap.Int("synced", synced))
	}
	return nil
}

// getSyncCursor reads the persisted cursor from the database. Returns a zero
// cursor (start of time) if no cursor has been stored yet.
func getSyncCursor(component core.Component) (slabs.Cursor, error) {
	var record models.ObjectSyncCursor
	if err := db.RetryableComponentLock(component, func(db *gorm.DB) *gorm.DB {
		return db.First(&record)
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			return slabs.Cursor{}, nil
		}
		return slabs.Cursor{}, err
	}
	if record.Cursor == "" {
		return slabs.Cursor{}, nil
	}
	var cursor slabs.Cursor
	if err := json.Unmarshal([]byte(record.Cursor), &cursor); err != nil {
		return slabs.Cursor{}, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}
	return cursor, nil
}

// setSyncCursor persists the cursor to the database (upsert).
func setSyncCursor(component core.Component, cursor slabs.Cursor) error {
	data, err := json.Marshal(cursor)
	if err != nil {
		return fmt.Errorf("failed to marshal cursor: %w", err)
	}
	cursorStr := string(data)
	return db.RetryableComponentLock(component, func(db *gorm.DB) *gorm.DB {
		// Upsert: if a row exists, update it; otherwise create it.
		var existing models.ObjectSyncCursor
		result := db.First(&existing)
		if result.Error == gorm.ErrRecordNotFound {
			return db.Create(&models.ObjectSyncCursor{Cursor: cursorStr})
		}
		return db.Model(&existing).Update("cursor", cursorStr)
	})
}
