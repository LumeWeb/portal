package testing

import (
	"bytes"
	"fmt"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.sia.tech/renterd/v2/api"
	"io"
	"sync"

	"github.com/stretchr/testify/mock"
)

// MockRenterService provides a higher-level abstraction for mocking the RenterService.
type MockRenterService struct {
	mockRenter *mocks.MockRenterService
	uploaded   map[string][]byte // bucket/filename -> data
	mu         sync.RWMutex
}

// NewMockRenterService creates a new MockRenterService.
func NewMockRenterService(t TB) *MockRenterService {
	mockRenter := mocks.NewMockRenterService(t)
	return &MockRenterService{
		mockRenter: mockRenter,
		uploaded:   make(map[string][]byte),
	}
}

// Mock returns the underlying MockRenterService.
func (h *MockRenterService) Mock() *mocks.MockRenterService {
	return h.mockRenter
}

// UploadObject mocks the UploadObject method and stores the uploaded data.
func (h *MockRenterService) UploadObject(bucket string, fileName string, data []byte, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)

	// Set expectation on the mock
	h.mockRenter.On("UploadObject",
		mock.AnythingOfType("*context.emptyCtx"),
		mock.MatchedBy(func(r io.Reader) bool {
			// Read the data from the reader
			buf := new(bytes.Buffer)
			_, readErr := buf.ReadFrom(r)
			if readErr != nil {
				return false // Indicate that the reader could not be read
			}
			return bytes.Equal(buf.Bytes(), data) // Compare the data
		}),
		bucket,
		fileName,
	).Return(err)

	if err == nil {
		h.uploaded[key] = data
	}
}

// GetObject mocks the GetObject method and returns the stored data.
func (h *MockRenterService) GetObject(bucket string, fileName string, data []byte, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", bucket, fileName)

	// Set expectation on the mock
	h.mockRenter.On("GetObject",
		mock.AnythingOfType("*context.emptyCtx"),
		bucket,
		fileName,
		api.DownloadObjectOptions{},
	).Return(&api.GetObjectResponse{
		Content: io.NopCloser(bytes.NewReader(data)),
	}, err)

	if err == nil {
		h.uploaded[key] = data
	}
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
func (h *MockRenterService) CreateBucketIfNotExists(bucket string, err error) {
	h.mockRenter.On("CreateBucketIfNotExists", bucket).Return(err)
}

// DeleteObject mocks the DeleteObject method.
func (h *MockRenterService) DeleteObject(bucket string, fileName string, err error) {
	h.mockRenter.On("DeleteObject", bucket, fileName).Return(err)
}

// UploadObjectMultipart mocks the UploadObjectMultipart method.
func (h *MockRenterService) UploadObjectMultipart(params *core.MultipartUploadParams, err error) {
	h.mockRenter.On("UploadObjectMultipart", mock.AnythingOfType("*context.emptyCtx"), params).Return(err)
}

// UploadExists mocks the UploadExists method.
func (h *MockRenterService) UploadExists(bucket string, fileName string, exists bool, upload *interface{}, err error) {
	h.mockRenter.On("UploadExists", mock.AnythingOfType("*context.emptyCtx"), bucket, fileName).Return(exists, upload, err)
}

// GetObjectMetadata mocks the GetObjectMetadata method.
func (h *MockRenterService) GetObjectMetadata(bucket string, fileName string, object *api.Object, err error) {
	h.mockRenter.On("GetObjectMetadata", mock.AnythingOfType("*context.emptyCtx"), bucket, fileName).Return(object, err)
}

// DeleteObjectMetadata mocks the DeleteObjectMetadata method.
func (h *MockRenterService) DeleteObjectMetadata(bucket string, fileName string, err error) {
	h.mockRenter.On("DeleteObjectMetadata", mock.AnythingOfType("*context.emptyCtx"), bucket, fileName).Return(err)
}

// UpdateGougingSettings mocks the UpdateGougingSettings method.
func (h *MockRenterService) UpdateGougingSettings(settings api.GougingSettings, err error) {
	h.mockRenter.On("UpdateGougingSettings", mock.AnythingOfType("*context.emptyCtx"), settings).Return(err)
}

// GougingSettings mocks the GougingSettings method.
func (h *MockRenterService) GougingSettings(settings api.GougingSettings, err error) {
	h.mockRenter.On("GougingSettings", mock.AnythingOfType("*context.emptyCtx")).Return(settings, err)
}

// SlabSize mocks the SlabSize method.
func (h *MockRenterService) SlabSize(size uint64, err error) {
	h.mockRenter.On("SlabSize", mock.AnythingOfType("*context.emptyCtx")).Return(size, err)
}

// AssertExpectations asserts that all expectations have been met.
func (h *MockRenterService) AssertExpectations(t TB) {
	h.mockRenter.AssertExpectations(t)
}
