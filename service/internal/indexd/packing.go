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
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// packingBatchSize caps the number of staged objects loaded per packing cycle
// to bound memory usage. Remaining objects are processed in subsequent ticks.
const packingBatchSize = 1000

// packingLoopInterval is how often the background packing loop runs.
const packingLoopInterval = 30 * time.Second

// PackingLoopCfg holds the dependencies and configuration for the packing loop.
type PackingLoopCfg struct {
	Component      core.Component
	Logger         *core.Logger
	SDK            SDK
	StagingBackend StagingBackend
	SlabSize       int64
}

// PackingLoop periodically batches staged small objects into packed uploads
// to fill slabs efficiently. It runs until ctx is cancelled.
func PackingLoop(ctx context.Context, cfg PackingLoopCfg, done chan struct{}) {
	defer close(done)

	// Recover objects stuck in "packing" from a previous crash/restart.
	// This must run before the first tick so that stale packing rows are
	// reprocessed immediately.
	if err := RecoverStuckPacking(ctx, cfg); err != nil {
		cfg.Logger.Error("failed to recover stuck packing objects", zap.Error(err))
	}

	t := time.NewTicker(packingLoopInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := PackStagedObjects(ctx, cfg); err != nil {
				cfg.Logger.Error("packing loop error", zap.Error(err))
			}
		}
	}
}

// RecoverStuckPacking reverts any objects left in "packing" state from a
// previous crash or restart, and finishes deleting objects stuck in
// "deleting" state. Since the packing loop only queries "staged"
// objects, both states would otherwise be orphaned forever.
func RecoverStuckPacking(ctx context.Context, cfg PackingLoopCfg) error {
	ctx, span := core.TraceMethod(ctx, "indexd.RecoverStuckPacking")
	defer span.End()
	// 1. Revert packing -> staged.
	var rowsAffected int64
	if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
		result := db.Model(&models.RenterObject{}).
			Where("status = ?", models.RenterObjectStatusPacking).
			Update("status", models.RenterObjectStatusStaged)
		rowsAffected = result.RowsAffected
		return result
	}); err != nil {
		return fmt.Errorf("failed to recover stuck packing objects: %w", err)
	}
	if rowsAffected > 0 {
		cfg.Logger.Info("recovered stuck packing objects",
			zap.Int64("count", rowsAffected),
		)
	}

	// 2. Finish deleting objects stuck in "deleting" state.
	// These rows had their SDK/staging data partially or fully cleaned up
	// before the crash. We attempt best-effort cleanup and remove the DB row.
	var deletingObjs []models.RenterObject
	if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", models.RenterObjectStatusDeleting).Find(&deletingObjs)
	}); err != nil {
		return fmt.Errorf("failed to query stuck deleting objects: %w", err)
	}
	for _, obj := range deletingObjs {
		if obj.SiaObjectID != "" {
			var id [32]byte
			n, err := hex.Decode(id[:], []byte(obj.SiaObjectID))
			if err != nil || n != 32 {
				cfg.Logger.Error("failed to parse sia_object_id for stuck deleting object",
					zap.String("siaObjectID", obj.SiaObjectID), zap.Error(err))
			} else {
				if derr := cfg.SDK.DeleteObject(ctx, id); derr != nil {
					cfg.Logger.Warn("failed to delete SDK object for stuck deleting row",
						zap.String("siaObjectID", obj.SiaObjectID), zap.Error(derr))
				}
			}
		}
		if obj.StagingKey != "" {
			if derr := cfg.StagingBackend.Delete(ctx, obj.StagingKey); derr != nil {
				cfg.Logger.Warn("failed to delete staging data for stuck deleting row",
					zap.String("stagingKey", obj.StagingKey), zap.Error(derr))
			}
		}
		if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
			return db.Unscoped().Delete(&obj)
		}); err != nil {
			cfg.Logger.Error("failed to delete stuck deleting row",
				zap.Uint("id", obj.ID), zap.Error(err))
		}
	}
	if len(deletingObjs) > 0 {
		cfg.Logger.Info("recovered stuck deleting objects",
			zap.Int("count", len(deletingObjs)),
		)
	}
	return nil
}

// PackStagedObjects queries for staged objects (Status=staged), CAS-transition
// them to packing, groups them into upload groups, and uploads each group via
// UploadPacked(). After upload, each object is pinned, sealed, and
// CAS-transitioned to uploaded. If an object was deleted during upload, the
// CAS to uploaded fails silently (RowsAffected=0) and the SDK object is
// cleaned up via DeleteObject.
func PackStagedObjects(ctx context.Context, cfg PackingLoopCfg) error {
	ctx, span := core.TraceMethod(ctx, "indexd.PackStagedObjects")
	defer span.End()
	// Query staged objects ordered by size descending for greedy packing.
	// Limit each cycle to cap memory usage; remaining objects are processed
	// in subsequent ticks.
	var staged []models.RenterObject
	if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).
			Where("status = ? AND staging_key != ?", models.RenterObjectStatusStaged, "").
			Order("size DESC").
			Limit(packingBatchSize).
			Find(&staged)
	}); err != nil {
		return fmt.Errorf("failed to query staged objects: %w", err)
	}

	if len(staged) == 0 {
		return nil
	}

	cfg.Logger.Debug("found staged objects for packing",
		zap.Int("count", len(staged)),
	)

	// Batch CAS transition: staged -> packing in a single query.
	// Collect the IDs of staged objects for the batch update.
	ids := make([]uint, 0, len(staged))
	for _, obj := range staged {
		ids = append(ids, obj.ID)
	}

	if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
		return db.Model(&models.RenterObject{}).
			Where("id IN ? AND status = ?", ids, models.RenterObjectStatusStaged).
			Update("status", models.RenterObjectStatusPacking)
	}); err != nil {
		return fmt.Errorf("failed to batch transition staged->packing: %w", err)
	}

	// Only keep objects that were successfully transitioned (status is now packing).
	var packingObjs []models.RenterObject
	if err := db.RetryableComponentLock(cfg.Component, func(db *gorm.DB) *gorm.DB {
		return db.Where("id IN ? AND status = ?", ids, models.RenterObjectStatusPacking).
			Order("size DESC").
			Find(&packingObjs)
	}); err != nil {
		return fmt.Errorf("failed to query packing objects: %w", err)
	}

	if len(packingObjs) == 0 {
		return nil
	}

	if skipped := len(staged) - len(packingObjs); skipped > 0 {
		cfg.Logger.Debug("skipped objects that failed staged->packing CAS",
			zap.Int("skipped", skipped),
		)
	}

	// Group objects into upload groups (~slabSize each).
	groups := groupStagedObjects(packingObjs, cfg.SlabSize, DefaultMaxGroupSize)

	for _, group := range groups {
		if err := uploadGroup(ctx, cfg, group); err != nil {
			cfg.Logger.Error("failed to upload group",
				zap.Int("objects", len(group)),
				zap.Error(err),
			)
			// Revert packing objects in this group back to staged via FSM.
			for _, obj := range group {
				if _, err := TransitionState(cfg.Component, ctx, obj.ID, models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); err != nil {
					cfg.Logger.Error("FSM revert packing->staged failed",
						zap.Uint("id", obj.ID),
						zap.Error(err),
					)
				}
			}
			// Continue to next group — failed groups will be retried next cycle.
		}
	}

	return nil
}

// DefaultMaxGroupSize is the maximum total size of a single upload group.
// Bounds the memory/time cost of a single packed upload.
const DefaultMaxGroupSize = 1 << 30 // 1 GiB

// groupStagedObjects groups staged objects into batches for packed upload.
// Each group accumulates objects until adding another would exceed the slab
// size or the max group size, then a new group starts.
func groupStagedObjects(objects []models.RenterObject, slabSize, maxGroupSize int64) [][]models.RenterObject {
	if len(objects) == 0 {
		return nil
	}

	var groups [][]models.RenterObject
	var current []models.RenterObject
	var currentSize int64

	for _, obj := range objects {
		if len(current) > 0 {
			// Start a new group if this object would exceed the slab size
			// or the max group size cap.
			wouldExceedSlab := currentSize+obj.Size > slabSize
			wouldExceedMax := currentSize+obj.Size > maxGroupSize
			if wouldExceedSlab || wouldExceedMax {
				groups = append(groups, current)
				current = nil
				currentSize = 0
			}
		}

		current = append(current, obj)
		currentSize += obj.Size
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}

// uploadGroup uploads a group of staged objects as a single packed upload.
// Each object's staging reader is added to the packed upload, finalized,
// and each resulting SDK object is pinned and sealed.
//
// If an Add() call fails mid-stream, the SDK leaves dead padding in the pipe
// (see siastorage packing.go:113-114). To handle this, on Add() failure we
// close the current packed upload, finalize what was successfully added, then
// start a new UploadPacked() for the remaining objects in the group.
//
// The objIdx slice tracks which original objects were successfully added, so
// Finalize results map back to the correct DB rows.
func uploadGroup(ctx context.Context, cfg PackingLoopCfg, group []models.RenterObject) error {
	ctx, span := core.TraceMethod(ctx, "indexd.uploadGroup")
	defer span.End()
	span.SetAttributes(
		attribute.Int("indexd.group.objectCount", len(group)),
	)
	var groupSize int64
	for _, obj := range group {
		groupSize += obj.Size
	}
	span.SetAttributes(attribute.Int64("indexd.group.totalSize", groupSize))
	// Process the group, splitting into sub-uploads on Add() failures.
	// Each sub-upload is finalized independently to avoid dead padding.
	return uploadSubGroup(ctx, cfg, group, 0)
}

// uploadSubGroup uploads objects[start:] as a packed upload. If Add() fails
// mid-stream, it finalizes the current upload, processes results, then
// continues with the remaining objects in a fresh UploadPacked().
//
// This is implemented iteratively (not recursively) to avoid stack overflow
// on large groups with repeated Add() failures. Each iteration creates a new
// UploadPacked handle, adds objects until failure, finalizes, processes
// results, then advances start past the failure point.
func uploadSubGroup(ctx context.Context, cfg PackingLoopCfg, group []models.RenterObject, start int) error {
	ctx, span := core.TraceMethod(ctx, "indexd.uploadSubGroup")
	defer span.End()
	span.SetAttributes(
		attribute.Int("indexd.subgroup.start", start),
		attribute.Int("indexd.subgroup.totalObjects", len(group)),
	)
	for start < len(group) {
		// Track consecutive staging read failures to bail early if the
		// staging backend is unavailable (Bug 4: avoids wasting SDK
		// round-trips on Finalize for an empty packed upload).
		consecutiveStagingFailures := 0
		const maxConsecutiveStagingFailures = 3

		upload, err := cfg.SDK.UploadPacked(ctx)
		if err != nil {
			return fmt.Errorf("failed to create packed upload: %w", err)
		}

		// Track which objects were successfully added.
		// Indices are relative to the original group slice.
		var objIdx []int
		splitAt := -1 // index where we should start a new sub-upload

		for i := start; i < len(group); i++ {
			obj := group[i]

			// Get the staged data reader.
			reader, err := cfg.StagingBackend.Get(ctx, obj.StagingKey, 0, -1)
			if err != nil {
				cfg.Logger.Error("failed to get staged object reader",
					zap.String("stagingKey", obj.StagingKey),
					zap.String("objectKey", obj.ObjectKey),
					zap.Error(err),
				)
				// Revert this object to staged so the next cycle retries it.
				if _, err := TransitionState(cfg.Component, ctx, obj.ID,
					models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); err != nil {
					cfg.Logger.Error("failed to revert object to staged after staging read failure",
						zap.Uint("id", obj.ID),
						zap.Error(err),
					)
				}
				consecutiveStagingFailures++
				if consecutiveStagingFailures >= maxConsecutiveStagingFailures {
					cfg.Logger.Warn("bailing out of group after consecutive staging read failures",
						zap.Int("failures", consecutiveStagingFailures),
						zap.Int("processed", len(objIdx)),
					)
					break
				}
				continue
			}
			consecutiveStagingFailures = 0

			n, err := upload.Add(ctx, reader)
			reader.Close()
			if err != nil {
				cfg.Logger.Error("failed to add object to packed upload, splitting group",
					zap.String("objectKey", obj.ObjectKey),
					zap.Error(err),
				)
				// Dead padding is now in the pipe. We must finalize this upload
				// (without the failed object) and start a new one for the rest.
				splitAt = i
				break
			}

			if n != obj.Size {
				cfg.Logger.Warn("unexpected bytes added to packed upload",
					zap.String("objectKey", obj.ObjectKey),
					zap.Int64("expected", obj.Size),
					zap.Int64("got", n),
				)
			}

			objIdx = append(objIdx, i)
		}

		// Bug 2: If no objects were added (e.g., first Add() failed or all
		// staging reads failed), skip Finalize entirely — calling Finalize on
		// an empty packed upload wastes an SDK round-trip and may create
		// garbage slabs on Sia.
		if len(objIdx) == 0 {
			upload.Close()
			if splitAt >= 0 && splitAt < len(group) {
				// Bug 3: Log CAS failure instead of silently ignoring it.
				rowsAffected, err := TransitionState(cfg.Component, ctx, group[splitAt].ID,
					models.RenterObjectStatusPacking, models.RenterObjectStatusStaged)
				if err != nil {
					cfg.Logger.Error("failed to revert object to staged after Add failure",
						zap.Uint("id", group[splitAt].ID),
						zap.Error(err),
					)
				} else if rowsAffected == 0 {
					cfg.Logger.Warn("CAS revert packing->staged affected 0 rows, object may be stuck",
						zap.Uint("id", group[splitAt].ID),
					)
				}
				start = splitAt + 1
				continue
			}
			// All remaining objects had staging read failures; they were
			// individually reverted. Nothing more to do.
			return nil
		}

		// Finalize the packed upload.
		results, err := upload.Finalize(ctx)
		upload.Close()
		if err != nil {
			// Objects may have been partially persisted; best-effort cleanup
			// is not possible here since we don't have the result IDs.
			return fmt.Errorf("failed to finalize packed upload: %w", err)
		}

		span.SetAttributes(attribute.Int("indexd.packed.resultsCount", len(results)))
		if len(results) != len(objIdx) {
			cfg.Logger.Error("finalize returned unexpected number of objects",
				zap.Int("expected", len(objIdx)),
				zap.Int("got", len(results)),
			)
			// Clean up the orphaned SDK objects that were finalized but
			// cannot be mapped back to DB rows.
			for _, obj := range results {
				if derr := cfg.SDK.DeleteObject(ctx, obj.ID()); derr != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object after count mismatch", zap.Error(derr))
				}
			}
			return fmt.Errorf("unexpected number of results: expected %d, got %d", len(objIdx), len(results))
		}

		// Pin and seal each object, CAS-transition to uploaded, clean up staging.
		// Following s3d's pattern: if MarkObjectUploaded fails with ErrObjectNotFound
		// (deleted during upload), skip and clean up the orphaned SDK object.
		for i, obj := range results {
			uploadObj := group[objIdx[i]]

			// Embed protocol/key metadata before pinning so the indexer can
			// attribute the object back to the portal.
			if err := SetObjectMetadata(&obj, uploadObj.Protocol, uploadObj.ObjectKey); err != nil {
				cfg.Logger.Error("failed to set object metadata",
					zap.String("objectKey", uploadObj.ObjectKey),
					zap.Error(err),
				)
				if _, rerr := TransitionState(cfg.Component, ctx, uploadObj.ID, models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); rerr != nil {
					cfg.Logger.Error("failed to revert packing->staged after metadata failure", zap.Uint("id", uploadObj.ID), zap.Error(rerr))
				}
				if derr := cfg.SDK.DeleteObject(ctx, obj.ID()); derr != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object after metadata failure", zap.Uint("id", uploadObj.ID), zap.Error(derr))
				}
				continue
			}

			// Pin the object.
			if err := cfg.SDK.PinObject(ctx, obj); err != nil {
				cfg.Logger.Error("failed to pin object",
					zap.String("objectKey", uploadObj.ObjectKey),
					zap.Error(err),
				)
				// Revert to staged so next cycle retries; clean up the
				// finalized (but unpinned) SDK object to avoid an orphan.
				if _, rerr := TransitionState(cfg.Component, ctx, uploadObj.ID, models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); rerr != nil {
					cfg.Logger.Error("failed to revert packing->staged after pin failure", zap.Uint("id", uploadObj.ID), zap.Error(rerr))
				}
				if derr := cfg.SDK.DeleteObject(ctx, obj.ID()); derr != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object after pin failure", zap.Uint("id", uploadObj.ID), zap.Error(derr))
				}
				continue
			}

			// Seal the object.
			sealed := cfg.SDK.SealObject(obj)
			sealedJSON, err := json.Marshal(sealed)
			if err != nil {
				cfg.Logger.Error("failed to marshal sealed object",
					zap.String("objectKey", uploadObj.ObjectKey),
					zap.Error(err),
				)
				if _, rerr := TransitionState(cfg.Component, ctx, uploadObj.ID, models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); rerr != nil {
					cfg.Logger.Error("failed to revert packing->staged after seal marshal failure", zap.Uint("id", uploadObj.ID), zap.Error(rerr))
				}
				if derr := cfg.SDK.DeleteObject(ctx, obj.ID()); derr != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object after seal marshal failure", zap.Uint("id", uploadObj.ID), zap.Error(derr))
				}
				continue
			}

			siaObjectID := obj.ID().String()

			// FSM CAS transition: packing -> uploaded with updates.
			// If rowsAffected=0, the object was deleted during upload.
			// Clean up the orphaned SDK object.
			rowsAffected, err := TransitionStateWithUpdates(cfg.Component, ctx, uploadObj.ID,
				models.RenterObjectStatusPacking, models.RenterObjectStatusUploaded,
				map[string]interface{}{
					"sia_object_id": siaObjectID,
					"sealed_data":   datatypes.JSON(sealedJSON),
					"staging_key":   "",
				})
			if err != nil {
				cfg.Logger.Error("FSM transition packing->uploaded failed, reverting to staged",
					zap.Uint("id", uploadObj.ID),
					zap.Error(err),
				)
				// Revert to staged so next cycle retries; clean up the orphaned SDK object.
				if _, revertErr := TransitionState(cfg.Component, ctx, uploadObj.ID, models.RenterObjectStatusPacking, models.RenterObjectStatusStaged); revertErr != nil {
					cfg.Logger.Error("failed to revert packing->staged", zap.Uint("id", uploadObj.ID), zap.Error(revertErr))
				}
				if delErr := cfg.SDK.DeleteObject(ctx, obj.ID()); delErr != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object on revert", zap.Uint("id", uploadObj.ID), zap.Error(delErr))
				}
				continue
			}

			if rowsAffected == 0 {
				// Object was deleted during upload. Clean up the orphaned SDK object.
				cfg.Logger.Warn("object was deleted during upload, cleaning up orphaned SDK object",
					zap.String("objectKey", uploadObj.ObjectKey),
					zap.String("siaObjectID", siaObjectID),
				)
				if err := cfg.SDK.DeleteObject(ctx, obj.ID()); err != nil {
					cfg.Logger.Error("failed to delete orphaned SDK object",
						zap.String("siaObjectID", siaObjectID),
						zap.Error(err),
					)
				}
				continue
			}

			// Clean up the staging data.
			if err := cfg.StagingBackend.Delete(ctx, uploadObj.StagingKey); err != nil {
				cfg.Logger.Warn("failed to delete staged object after packing",
					zap.String("stagingKey", uploadObj.StagingKey),
					zap.Error(err),
				)
			}
		}

		// If we split due to an Add() failure, revert the failed object to
		// staged and continue with the remaining objects in a new
		// UploadPacked() handle.
		if splitAt >= 0 && splitAt < len(group) {
			rowsAffected, err := TransitionState(cfg.Component, ctx, group[splitAt].ID,
				models.RenterObjectStatusPacking, models.RenterObjectStatusStaged)
			if err != nil {
				cfg.Logger.Error("failed to revert object to staged after Add failure (split)",
					zap.Uint("id", group[splitAt].ID),
					zap.Error(err),
				)
			} else if rowsAffected == 0 {
				cfg.Logger.Warn("CAS revert packing->staged affected 0 rows after split, object may be stuck",
					zap.Uint("id", group[splitAt].ID),
				)
			}
			cfg.Logger.Info("continuing packed upload in new sub-group",
				zap.Int("remaining", len(group)-splitAt-1),
			)
			start = splitAt + 1
			continue
		}

		// No split — all objects in [start, len(group)) were processed.
		return nil
	}

	return nil
}

// StopPackingLoop signals the packing loop to stop and waits for it to exit.
func StopPackingLoop(cancel context.CancelFunc, done chan struct{}, logger *core.Logger) {
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			logger.Warn("packing loop did not stop within timeout")
		}
	}
}
