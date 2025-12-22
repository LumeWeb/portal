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

// TUSUploadReader wraps a TUS upload reader to provide io.ReadSeekCloser functionality
// using the modern ContentServerDataStore approach for S3-backed uploads
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
	// Check for context cancellation
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

	// Calculate remaining bytes from current position to end
	remainingBytes := r.info.Size - r.position
	if remainingBytes <= 0 {
		return 0, io.EOF
	}

	// Don't read more than we have space for
	maxRead := int64(len(p))
	if remainingBytes < maxRead {
		maxRead = remainingBytes
	}

	// Calculate range for this read request (maxRead is already constrained by remainingBytes)
	rangeEnd := r.position + maxRead - 1

	// Create HTTP request for range
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set range header
	rangeHeader := fmt.Sprintf("bytes=%d-%d", r.position, rangeEnd)
	req.Header.Set("Range", rangeHeader)

	// Create response recorder to capture the response
	recorder := newResponseRecorder()

	// Use ServeContent to get the data
	err = r.servableUpload.ServeContent(r.ctx, recorder, req)
	if err != nil {
		return 0, fmt.Errorf("failed to serve content: %w", err)
	}

	// Handle partial content responses
	if recorder.statusCode == http.StatusPartialContent || recorder.statusCode == http.StatusOK {
		data := recorder.buffer.Bytes()

		// Calculate how many bytes will actually be copied
		copied := min(len(data), len(p))
		copy(p[:copied], data[:copied])

		// Advance position only by bytes actually copied
		r.position += int64(copied)
		n = copied

		// Check for EOF after position update
		if recorder.statusCode == http.StatusOK && r.position >= r.info.Size {
			return n, io.EOF
		}

		return n, nil
	} else if recorder.statusCode == http.StatusRequestedRangeNotSatisfiable {
		// Range not satisfiable - this means we tried to read beyond the file
		return 0, io.EOF
	} else {
		return 0, fmt.Errorf("unexpected status code: %d", recorder.statusCode)
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
