package testing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
	sdk "go.sia.tech/siastorage"

	"go.lumeweb.com/portal/service"
	"go.lumeweb.com/portal/service/internal/indexd"
)

// This file provides hand-written mocks for the indexd.SDK, indexd.PackedUploader,
// and indexd.StagingBackend interfaces. They are hand-written (not mockery-generated)
// because the interfaces live in the internal package service/internal/indexd,
// which mockery cannot reach from outside the service module.
//
// Tests in core/internal/service_tests/ import these mocks to test the public
// service.RenterService API without a real Sia network.

// --- Re-exported interface types ---
// These type aliases let external test packages satisfy the internal interfaces.

// RenterSDK is the indexd.SDK interface re-exported for testing.
type RenterSDK = indexd.SDK

// RenterPackedUploader is the indexd.PackedUploader interface re-exported for testing.
type RenterPackedUploader = indexd.PackedUploader

// RenterStagingBackend is the indexd.StagingBackend interface re-exported for testing.
type RenterStagingBackend = indexd.StagingBackend

// --- Mock implementations ---

// MockRenterSDK is a mock indexd.SDK for renter service tests.
type MockRenterSDK struct {
	Upload               *MockRenterPackedUpload
	UploadErr            error
	UploadOpts           []sdk.UploadOption
	UploadFactory        func() *MockRenterPackedUpload // if set, UploadPacked calls this to create a fresh upload
	PinErr               error
	DeleteErr            error
	PinnedObjs           []sdk.Object
	DeletedObjs          []types.Hash256
	DownloadData         []byte
	DownloadErr          error
	DownloadOpts         []sdk.DownloadOption
	UnsealObj            sdk.Object
	UnsealErr            error
	SealObj              sdk.SealedObject
	ObjectEventsResult   []sdk.ObjectEvent
	ObjectEventsErr      error
}

func (m *MockRenterSDK) UploadPacked(_ context.Context, opts ...sdk.UploadOption) (indexd.PackedUploader, error) {
	m.UploadOpts = opts
	if m.UploadErr != nil {
		return nil, m.UploadErr
	}
	if m.UploadFactory != nil {
		return m.UploadFactory(), nil
	}
	return m.Upload, nil
}

func (m *MockRenterSDK) Download(_ context.Context, obj sdk.Object, opts ...sdk.DownloadOption) (io.ReadCloser, error) {
	m.DownloadOpts = opts
	if m.DownloadErr != nil {
		return nil, m.DownloadErr
	}
	return io.NopCloser(bytes.NewReader(m.DownloadData)), nil
}

func (m *MockRenterSDK) PinObject(_ context.Context, obj sdk.Object) error {
	m.PinnedObjs = append(m.PinnedObjs, obj)
	return m.PinErr
}

func (m *MockRenterSDK) DeleteObject(_ context.Context, key [32]byte) error {
	m.DeletedObjs = append(m.DeletedObjs, key)
	return m.DeleteErr
}

func (m *MockRenterSDK) SealObject(_ sdk.Object) sdk.SealedObject {
	return m.SealObj
}

func (m *MockRenterSDK) UnsealObject(_ sdk.SealedObject) (sdk.Object, error) {
	return m.UnsealObj, m.UnsealErr
}

func (m *MockRenterSDK) ObjectEvents(_ context.Context, _ slabs.Cursor, _ int) ([]sdk.ObjectEvent, error) {
	return m.ObjectEventsResult, m.ObjectEventsErr
}

// MockRenterPackedUpload is a mock indexd.PackedUploader.
type MockRenterPackedUpload struct {
	AddedData      [][]byte
	AddErr         error
	AddFailOnCalls []int // Add returns AddErr on these call numbers (1-indexed)
	FinalizeErr    error
	FinalizeObjs   []sdk.Object // if set, returned by Finalize instead of auto-generated
	Objects        []sdk.Object
	Closed         bool
	AddCallCount   int
	FinalizeCount  int
	AddFailOnCall  int // if >0, Add returns AddErr on this call number (1-indexed)
}

func (m *MockRenterPackedUpload) Add(_ context.Context, r io.Reader) (int64, error) {
	if m.Closed {
		return 0, errors.New("upload closed")
	}
	m.AddCallCount++
	// Check single-call failure
	if m.AddFailOnCall > 0 && m.AddCallCount == m.AddFailOnCall {
		return 0, m.AddErr
	}
	// Check multi-call failures
	for _, failCall := range m.AddFailOnCalls {
		if m.AddCallCount == failCall {
			return 0, m.AddErr
		}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	m.AddedData = append(m.AddedData, data)
	return int64(len(data)), nil
}

func (m *MockRenterPackedUpload) Finalize(_ context.Context) ([]sdk.Object, error) {
	m.FinalizeCount++
	if m.FinalizeErr != nil {
		return nil, m.FinalizeErr
	}
	if m.FinalizeObjs != nil {
		return m.FinalizeObjs, nil
	}
	if m.Objects != nil {
		return m.Objects, nil
	}
	// Return one mock object per added chunk.
	n := len(m.AddedData)
	objs := make([]sdk.Object, n)
	for i := range objs {
		objs[i] = sdk.NewEmptyObject()
	}
	return objs, nil
}

func (m *MockRenterPackedUpload) Close() error {
	m.Closed = true
	return nil
}

// MockStagingBackend is an in-memory indexd.StagingBackend for tests.
type MockStagingBackend struct {
	Data      map[string][]byte
	NextID    int
	PutErr    error
	GetErr    error // if set, Get always returns this error
	DeleteErr error
}

func NewMockStagingBackend() *MockStagingBackend {
	return &MockStagingBackend{Data: make(map[string][]byte)}
}

func (m *MockStagingBackend) Put(_ context.Context, reader io.Reader) (string, error) {
	if m.PutErr != nil {
		return "", m.PutErr
	}
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("staging/%d", m.NextID)
	m.NextID++
	m.Data[key] = buf
	return key, nil
}

func (m *MockStagingBackend) Get(_ context.Context, stagingKey string, offset, length int64) (io.ReadCloser, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	data, ok := m.Data[stagingKey]
	if !ok {
		return nil, fmt.Errorf("staging key not found: %s", stagingKey)
	}
	if offset > 0 {
		if int64(len(data)) < offset {
			return nil, fmt.Errorf("offset out of range")
		}
		data = data[offset:]
	}
	if length >= 0 && int64(len(data)) > length {
		data = data[:length]
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockStagingBackend) Delete(_ context.Context, stagingKey string) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	delete(m.Data, stagingKey)
	return nil
}

func (m *MockStagingBackend) Size(_ context.Context, stagingKey string) (int64, error) {
	data, ok := m.Data[stagingKey]
	if !ok {
		return 0, fmt.Errorf("staging key not found: %s", stagingKey)
	}
	return int64(len(data)), nil
}

// --- Test helpers ---

// SetupRenterService creates a RenterService wired with mock SDK and staging backend,
// registered in the test context. Returns the service, mock SDK, and mock staging backend.
func SetupRenterService(slabSize int64) (*service.RenterService, *MockRenterSDK, *MockStagingBackend) {
	mockSDK := &MockRenterSDK{
		Upload: &MockRenterPackedUpload{},
	}
	mockStaging := NewMockStagingBackend()
	renter := &service.RenterService{}
	renter.SetSDK(mockSDK)
	renter.SetStagingBackend(mockStaging)
	renter.SetSlabSize(slabSize)
	return renter, mockSDK, mockStaging
}
