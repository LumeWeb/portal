package migrator

import (
	"context"
	"fmt"
	"io"
	"strings"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// MigrationStats tracks the outcome of a single migration run.
type MigrationStats struct {
	Total    int // total objects examined from renterd
	Migrated int // objects successfully re-uploaded to indexd
	Skipped  int // objects already present in renter_objects (by protocol+key)
	Errors   int // objects that encountered an error
}

// String returns a human-readable summary.
func (s MigrationStats) String() string {
	return fmt.Sprintf("total=%d migrated=%d skipped=%d errors=%d",
		s.Total, s.Migrated, s.Skipped, s.Errors)
}

// Lister is the subset of RenterdClient needed for listing objects.
type Lister interface {
	ListAllObjects(ctx context.Context, bucket string) ([]RenterdObjectMetadata, error)
}

// Downloader is the subset of RenterdClient needed for downloading objects.
type Downloader interface {
	DownloadObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// Migrator orchestrates the migration of objects from a renterd backend
// to the indexd-native backend by re-uploading each object through the
// existing RenterService interface.
type Migrator struct {
	Renter     core.RenterService
	Lister     Lister
	Downloader Downloader
	Logger     *core.Logger
	DryRun     bool
}

// Migrate iterates all registered storage protocols, lists objects from
// renterd for each protocol's bucket, downloads each object, and re-uploads
// it through the RenterService (which handles UploadPacked, pin, seal, and
// DB storage). The migration is idempotent: RenterService.UploadObject
// already skips objects that exist.
func (m *Migrator) Migrate(ctx context.Context, protocols []core.StorageProtocol) (MigrationStats, error) {
	var stats MigrationStats

	for _, protocol := range protocols {
		bucket := protocol.Name()

		objects, err := m.Lister.ListAllObjects(ctx, bucket)
		if err != nil {
			m.Logger.Error("failed to list objects from renterd",
				zap.String("bucket", bucket),
				zap.Error(err),
			)
			stats.Errors++
			continue
		}

		m.Logger.Info("listed objects from renterd",
			zap.String("bucket", bucket),
			zap.Int("count", len(objects)),
		)

		for _, obj := range objects {
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			default:
			}

			stats.Total++
			// Renterd keys may have a leading slash (e.g. /Qm...).
			// Normal uploads use EncodeFileName which has no leading slash.
			// Strip it so keys match existing objects and DB lookups.
			objectKey := strings.TrimPrefix(obj.Key, "/")

			if m.DryRun {
				m.Logger.Info("[dry-run] would migrate object",
					zap.String("bucket", bucket),
					zap.String("objectKey", objectKey),
					zap.Int64("size", obj.Size),
				)
				continue
			}

			// The objectKey from renterd IS the encoded file name
			// (protocol.EncodeFileName(hash)). Check if it's already uploaded.
			exists, _, err := m.Renter.UploadExists(ctx, bucket, objectKey)
			if err != nil {
				stats.Errors++
				m.Logger.Error("failed to check if object exists",
					zap.String("bucket", bucket),
					zap.String("objectKey", objectKey),
					zap.Error(err),
				)
				continue
			}
			if exists {
				stats.Skipped++
				m.Logger.Debug("object already uploaded, skipping",
					zap.String("bucket", bucket),
					zap.String("objectKey", objectKey),
				)
				continue
			}

			if err := m.migrateObject(ctx, protocol, bucket, objectKey, obj.Size); err != nil {
				stats.Errors++
				m.Logger.Error("failed to migrate object",
					zap.String("bucket", bucket),
					zap.String("objectKey", objectKey),
					zap.Error(err),
				)
				continue
			}
			stats.Migrated++
		}
	}

	m.Logger.Info("migration complete", zap.String("stats", stats.String()))
	return stats, nil
}

// migrateObject downloads a single object from renterd and re-uploads it
// through the RenterService, which handles UploadPacked, pin, seal, and
// DB storage. The objectKey from renterd is already the encoded file name.
func (m *Migrator) migrateObject(ctx context.Context, protocol core.StorageProtocol, bucket, objectKey string, size int64) error {
	rc, err := m.Downloader.DownloadObject(ctx, bucket, objectKey)
	if err != nil {
		return fmt.Errorf("download from renterd: %w", err)
	}
	defer rc.Close()

	// Compute hash for the DB record — same as a normal upload would.
	hash, err := protocol.Hash(rc, uint64(size))
	if err != nil {
		return fmt.Errorf("compute storage hash: %w", err)
	}

	// Hash consumed the reader — re-download for the actual upload.
	rc.Close()
	rc, err = m.Downloader.DownloadObject(ctx, bucket, objectKey)
	if err != nil {
		return fmt.Errorf("re-download from renterd: %w", err)
	}
	defer rc.Close()

	if err := m.Renter.UploadObject(ctx, rc, bucket, objectKey, hash.Bytes()); err != nil {
		return fmt.Errorf("upload via renter service: %w", err)
	}

	m.Logger.Info("migrated object",
		zap.String("bucket", bucket),
		zap.String("objectKey", objectKey),
		zap.Int64("size", size),
	)

	return nil
}
