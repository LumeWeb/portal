package tus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/tus/tusd/v2/pkg/handler"
	"go.lumeweb.com/portal/core"
)

// TUSUploadReader wraps a TUS upload reader to provide io.ReadSeekCloser and
// io.ReaderAt functionality using the ContentServerDataStore approach for
// S3-backed uploads
type TUSUploadReader struct {
	mu             sync.RWMutex
	ctx            context.Context
	logger         *core.Logger
	upload         handler.Upload
	servableUpload handler.ServableUpload
	info           handler.FileInfo
	position       int64
	closed         bool
}

// NewTUSUploadReader creates a new seekable reader for a TUS upload
func NewTUSUploadReader(
	ctx context.Context,
	logger *core.Logger,
	upload handler.Upload,
	info handler.FileInfo,
	startOffset int64,
) (*TUSUploadReader, error) {
	ctx, span := core.TraceMethod(ctx, "NewTUSUploadReader")
	defer span.End()

	if logger == nil {
		return nil, errors.New("logger cannot be nil")
	}
	if upload == nil {
		return nil, errors.New("upload cannot be nil")
	}
	if startOffset < 0 {
		return nil, fmt.Errorf("start offset cannot be negative: %d", startOffset)
	}
	if startOffset > info.Size {
		return nil, fmt.Errorf("start offset %d exceeds upload size %d", startOffset, info.Size)
	}

	// Try to get the ServableUpload interface from the upload
	var servableUpload handler.ServableUpload
	var ok bool
	if servableUpload, ok = upload.(handler.ServableUpload); !ok {
		// Fall back to the old approach if ContentServerDataStore is not implemented
		return nil, errors.New("upload does not implement ContentServerDataStore interface")
	}

	reader := &TUSUploadReader{
		ctx:            ctx,
		logger:         logger,
		upload:         upload,
		servableUpload: servableUpload,
		info:           info,
		position:       startOffset,
	}

	return reader, nil
}

// Read implements io.Reader using the modern ContentServerDataStore approach
func (r *TUSUploadReader) Read(p []byte) (n int, err error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	remainingBytes := r.info.Size - r.position
	if remainingBytes <= 0 {
		return 0, io.EOF
	}

	maxRead := int64(len(p))
	if remainingBytes < maxRead {
		maxRead = remainingBytes
	}

	rangeEnd := r.position + maxRead - 1

	result, err := r.serveRange(r.position, rangeEnd)
	if err != nil {
		return 0, err
	}

	data := result.data
	if result.statusCode == http.StatusOK {
		if r.position >= int64(len(data)) {
			return 0, io.EOF
		}
		end := int64(len(data))
		if r.position+maxRead < end {
			end = r.position + maxRead
		}
		data = data[r.position:end]
	}

	copied := min(len(data), len(p))
	copy(p[:copied], data[:copied])
	r.position += int64(copied)

	if r.position >= r.info.Size {
		return copied, io.EOF
	}

	return copied, nil
}

// ReadAt implements io.ReaderAt — reads from offset without affecting position.
// Safe for concurrent use from multiple goroutines.
func (r *TUSUploadReader) ReadAt(p []byte, off int64) (n int, err error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	r.mu.RLock()
	closed := r.closed
	size := r.info.Size
	r.mu.RUnlock()

	if closed {
		return 0, io.EOF
	}

	if off < 0 {
		return 0, fmt.Errorf("negative offset: %d", off)
	}

	if off >= size {
		return 0, io.EOF
	}

	remainingBytes := size - off
	maxRead := int64(len(p))
	if remainingBytes < maxRead {
		maxRead = remainingBytes
	}

	rangeEnd := off + maxRead - 1

	result, err := r.serveRange(off, rangeEnd)
	if err != nil {
		return 0, err
	}

	data := result.data
	if result.statusCode == http.StatusOK {
		if off >= int64(len(data)) {
			return 0, io.EOF
		}
		end := int64(len(data))
		if off+maxRead < end {
			end = off + maxRead
		}
		data = data[off:end]
	}

	copied := min(len(data), len(p))
	copy(p[:copied], data[:copied])

	if off+int64(copied) >= size {
		return copied, io.EOF
	}

	return copied, nil
}

// serveRangeResult holds the data and status from a ServeContent call.
type serveRangeResult struct {
	data       []byte
	statusCode int
}

// serveRange fetches bytes [start, end] via ServeContent. Must not be called
// while holding r.mu (Read holds the write lock; ReadAt does not hold it).
func (r *TUSUploadReader) serveRange(start, end int64) (*serveRangeResult, error) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	recorder := newResponseRecorder()

	if err := r.servableUpload.ServeContent(r.ctx, recorder, req); err != nil {
		return nil, fmt.Errorf("failed to serve content: %w", err)
	}

	switch recorder.statusCode {
	case http.StatusPartialContent, http.StatusOK:
		return &serveRangeResult{data: recorder.buffer.Bytes(), statusCode: recorder.statusCode}, nil
	case http.StatusRequestedRangeNotSatisfiable:
		return nil, io.EOF
	default:
		return nil, fmt.Errorf("unexpected status code: %d", recorder.statusCode)
	}
}

// Seek implements io.Seeker for the ContentServerDataStore approach
func (r *TUSUploadReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("cannot seek on closed reader")
	}

	var newPosition int64

	switch whence {
	case io.SeekStart:
		newPosition = offset
	case io.SeekCurrent:
		newPosition = r.position + offset
	case io.SeekEnd:
		newPosition = r.info.Size + offset
	default:
		return 0, errors.New("unsupported whence value")
	}

	// Validate new position
	if newPosition < 0 {
		return 0, fmt.Errorf("cannot seek to negative position %d", newPosition)
	}
	if newPosition > r.info.Size {
		return 0, fmt.Errorf("cannot seek beyond upload size %d", r.info.Size)
	}

	r.position = newPosition
	return r.position, nil
}

// Close implements io.Closer
func (r *TUSUploadReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	return nil
}

// responseRecorder captures HTTP response data for the ServeContent method
type responseRecorder struct {
	statusCode int
	headers    http.Header
	buffer     *bytes.Buffer
}

// newResponseRecorder creates a new responseRecorder instance
func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		statusCode: http.StatusOK, // Default to 200, matching http.ResponseWriter behavior
		headers:    make(http.Header),
		buffer:     bytes.NewBuffer(nil),
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	return r.buffer.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
