package tus

import (
	"context"
	"fmt"
	"io"
	"sync"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const (
	// DefaultUpdateChunkSize is the default chunk size (in bytes) for updating workflow data
	DefaultUpdateChunkSize = 1024 * 1024 // 1MB
)

// HashProgressReader wraps an io.Reader to track progress during hashing operations
// and updates workflow data at chunk boundaries to avoid excessive database writes.
type HashProgressReader struct {
	reader         io.Reader
	totalBytes     int64
	bytesRead      int64
	updateChunk    int64
	lastUpdateSize int64
	requestID      uint
	workflowSvc    core.WorkflowService
	logger         *core.Logger
	mu             sync.Mutex
}

// NewHashProgressReader creates a new HashProgressReader that wraps the provided reader.
//
// Parameters:
//   - reader: The underlying reader to read from
//   - totalBytes: Total size of the data being hashed
//   - requestID: The request ID for updating workflow data
//   - workflowSvc: Workflow service for updating progress
//   - logger: Logger for debugging
//   - updateChunk: Chunk size (in bytes) for workflow data updates. If 0, uses DefaultUpdateChunkSize.
func NewHashProgressReader(
	reader io.Reader,
	totalBytes int64,
	requestID uint,
	workflowSvc core.WorkflowService,
	logger *core.Logger,
	updateChunk int64,
) *HashProgressReader {
	if updateChunk <= 0 {
		updateChunk = DefaultUpdateChunkSize
	}

	return &HashProgressReader{
		reader:       reader,
		totalBytes:   totalBytes,
		bytesRead:    0,
		updateChunk:  updateChunk,
		requestID:    requestID,
		workflowSvc:  workflowSvc,
		logger:       logger,
	}
}

// Read implements io.Reader. It reads from the underlying reader and updates
// workflow data at chunk boundaries.
func (h *HashProgressReader) Read(p []byte) (n int, err error) {
	n, err = h.reader.Read(p)
	if n > 0 {
		h.mu.Lock()
		h.bytesRead += int64(n)

		// Update workflow data if we've crossed a chunk boundary
		if h.bytesRead-h.lastUpdateSize >= h.updateChunk {
			h.updateProgress(context.Background())
			h.lastUpdateSize = h.bytesRead
		}

		h.mu.Unlock()
	}

	return n, err
}

// updateProgress updates the workflow data with the current hashing progress.
// This is called at chunk boundaries to avoid excessive database writes.
func (h *HashProgressReader) updateProgress(ctx context.Context) {
	if h.workflowSvc == nil || h.requestID == 0 {
		return
	}

	progressPercent := float64(0)
	if h.totalBytes > 0 {
		progressPercent = float64(h.bytesRead) / float64(h.totalBytes) * 100
	}

	data := map[string]any{
		"hash_progress_bytes": h.bytesRead,
		"hash_progress_total": h.totalBytes,
	}

	err := h.workflowSvc.UpdateWorkflowData(ctx, h.requestID, data)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("Failed to update hashing progress in workflow data",
				zap.Error(err),
				zap.Uint("requestID", h.requestID),
				zap.Int64("bytesRead", h.bytesRead),
				zap.Int64("totalBytes", h.totalBytes),
			)
		}
	}
}

// Finalize updates workflow data with the final progress (100% complete).
// This should be called after hashing completes successfully.
func (h *HashProgressReader) Finalize(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.bytesRead = h.totalBytes
	data := map[string]any{
		"hash_progress_bytes":    h.bytesRead,
		"hash_progress_total":    h.totalBytes,
		"hash_progress_complete": true,
	}

	if h.workflowSvc != nil && h.requestID != 0 {
		err := h.workflowSvc.UpdateWorkflowData(ctx, h.requestID, data)
		if err != nil && h.logger != nil {
			h.logger.Warn("Failed to finalize hashing progress in workflow data",
				zap.Error(err),
				zap.Uint("requestID", h.requestID),
			)
		}
	}
}

// BytesRead returns the number of bytes read so far.
func (h *HashProgressReader) BytesRead() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bytesRead
}

// TotalBytes returns the total bytes expected.
func (h *HashProgressReader) TotalBytes() int64 {
	return h.totalBytes
}

// ProgressPercent returns the current progress as a percentage (0-100).
func (h *HashProgressReader) ProgressPercent() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.totalBytes == 0 {
		return 0
	}
	return float64(h.bytesRead) / float64(h.totalBytes) * 100
}
