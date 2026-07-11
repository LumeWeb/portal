package testing

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.RenterService = (*MockRenterService)(nil)

// MockRenterService provides a higher-level abstraction for mocking the RenterService.
type MockRenterService struct {
	uploaded map[string][]byte // bucket/filename -> data
	buckets  map[string]bool   // track created buckets
	mu       sync.RWMutex
	t        testing.TB
	componentConfig  config.Manager
	componentContext core.Context
	componentLogger  *core.Logger
	componentDB      *gorm.DB
}

func (h *MockRenterService) ID() string {
	return core.RENTER_SERVICE
}

// NewMockRenterService creates a new MockRenterService.
func NewMockRenterService(t TB) *MockRenterService {
	return &MockRenterService{
		uploaded: make(map[string][]byte),
		buckets:  make(map[string]bool),
		t:        t,
	}
}

// UploadObject mocks the UploadObject method and stores the uploaded data.
func (h *MockRenterService) UploadObject(_ context.Context, file io.Reader, bucket string, fileName string, _ []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)

	// Read the data from the reader
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	h.uploaded[key] = data
	return nil
}

// GetObject mocks the GetObject method and returns the stored data.
func (h *MockRenterService) GetObject(_ context.Context, bucket string, fileName string, options core.DownloadOptions) (io.ReadCloser, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	data, exists := h.uploaded[key]
	if !exists {
		return nil, fmt.Errorf("object not found")
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// AssertUploadExists asserts that an upload exists for the given bucket and filename.
func (h *MockRenterService) AssertUploadExists(bucket string, fileName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	_, ok := h.uploaded[key]
	return ok
}

// GetUploadedData returns the uploaded data for the given bucket and filename.
func (h *MockRenterService) GetUploadedData(bucket string, fileName string) []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	return h.uploaded[key]
}

// CreateBucketIfNotExists mocks the CreateBucketIfNotExists method.
func (h *MockRenterService) CreateBucketIfNotExists(bucket string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.buckets[bucket] = true
	return nil
}

// DeleteObject mocks the DeleteObject method.
func (h *MockRenterService) DeleteObject(_ context.Context, bucket string, fileName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	delete(h.uploaded, key)

	return nil
}

// UploadObjectMultipart mocks the UploadObjectMultipart method.
func (h *MockRenterService) UploadObjectMultipart(_ context.Context, params *core.MultipartUploadParams) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Simulate storing the uploaded data
	if params != nil && params.ReaderFactory != nil {
		reader, err := params.ReaderFactory(0, uint(params.Size))
		if err != nil {
			return err
		}
		defer func(reader io.ReadCloser) {
			err = reader.Close()
			if err != nil {
				h.t.Errorf("failed to close multipart upload reader: %v", err)
			}
		}(reader)

		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		key := fmt.Sprintf("%s/%s", params.Bucket, params.FileName)
		h.uploaded[key] = data
	}

	return nil
}

// UploadExists mocks the UploadExists method.
func (h *MockRenterService) UploadExists(_ context.Context, bucket string, fileName string) (bool, *models.RenterObject, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	_, exists := h.uploaded[key]

	return exists, &models.RenterObject{
		SiaObjectID: fileName,
		Bucket:      bucket,
		ObjectKey:   fileName,
	}, nil
}

// GetObjectMetadata mocks the GetObjectMetadata method.
func (h *MockRenterService) GetObjectMetadata(_ context.Context, bucket string, fileName string) (*core.ObjectMetadata, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	data, exists := h.uploaded[key]
	if !exists {
		return nil, fmt.Errorf("object not found")
	}

	return &core.ObjectMetadata{
		Bucket:  bucket,
		Key:     fileName,
		Size:    int64(len(data)),
		ETag:    fmt.Sprintf("%x", md5.Sum(data)),
		ModTime: time.Now(),
	}, nil
}

// BucketExists returns whether a bucket has been created
func (h *MockRenterService) BucketExists(bucket string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.buckets[bucket]
}

// DeleteObjectMetadata mocks the DeleteObjectMetadata method.
func (h *MockRenterService) DeleteObjectMetadata(_ context.Context, bucket string, fileName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)
	delete(h.uploaded, key)

	return nil
}

// SlabSize mocks the SlabSize method.
func (h *MockRenterService) SlabSize(_ context.Context) (uint64, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Default slab size of 4MB
	return uint64(4 * 1024 * 1024), nil
}

// Config implements core.Component
func (h *MockRenterService) Config() config.Manager {
	return h.componentConfig
}

// SetConfig implements core.Component
func (h *MockRenterService) SetConfig(cfg config.Manager) {
	h.componentConfig = cfg
}

// Context implements core.Component
func (h *MockRenterService) Context() core.Context {
	return h.componentContext
}

// SetContext implements core.Component
func (h *MockRenterService) SetContext(ctx core.Context) {
	h.componentContext = ctx
}

// Logger implements core.Component
func (h *MockRenterService) Logger() *core.Logger {
	return h.componentLogger
}

// SetLogger implements core.Component
func (h *MockRenterService) SetLogger(logger *core.Logger) {
	h.componentLogger = logger
}

// DB implements core.Component
func (h *MockRenterService) DB() *gorm.DB {
	return h.componentDB
}

// SetDB implements core.Component
func (h *MockRenterService) SetDB(db *gorm.DB) {
	h.componentDB = db
}
