package indexd

import (
	"context"
	"io"
)

// StagingBackend stores small objects pending pack/upload to Sia.
// Implementations: S3 (default), disk (future), memory (testing).
type StagingBackend interface {
	// Put stores object data in staging, returns a staging key for later retrieval.
	Put(ctx context.Context, reader io.Reader) (stagingKey string, err error)
	// Get returns a reader for staged object data. offset/length support range reads.
	// offset=0, length=-1 means read all.
	Get(ctx context.Context, stagingKey string, offset, length int64) (io.ReadCloser, error)
	// Delete removes staged object data.
	Delete(ctx context.Context, stagingKey string) error
	// Size returns the byte size of staged data.
	Size(ctx context.Context, stagingKey string) (int64, error)
}
