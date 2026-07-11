package indexd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
	sdk "go.sia.tech/siastorage"
	"gorm.io/gorm"
)

// --- Mock implementations ---

// mockPackedUpload is a mock PackedUploader for testing.
type mockPackedUpload struct {
	objects      []sdk.Object
	addedData    [][]byte
	addErrOnIdx  int // -1 = no error
	finalizeErr  error
	finalizeVals []sdk.Object // override return value
	closed       bool
}

func newMockPackedUpload(objects []sdk.Object, addErrOnIdx int) *mockPackedUpload {
	return &mockPackedUpload{
		objects:     objects,
		addErrOnIdx: addErrOnIdx,
	}
}

func (m *mockPackedUpload) Add(_ context.Context, r io.Reader) (int64, error) {
	if m.closed {
		return 0, errors.New("upload closed")
	}
	idx := len(m.addedData)
	if m.addErrOnIdx >= 0 && idx == m.addErrOnIdx {
		// Simulate the dead-padding scenario: read and discard, then error.
		io.Copy(io.Discard, r)
		// Mark that this index errored so subsequent adds proceed.
		m.addErrOnIdx = -1
		return 0, errors.New("mock add error")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	m.addedData = append(m.addedData, data)
	return int64(len(data)), nil
}

func (m *mockPackedUpload) Finalize(_ context.Context) ([]sdk.Object, error) {
	if m.finalizeErr != nil {
		return nil, m.finalizeErr
	}
	if m.finalizeVals != nil {
		return m.finalizeVals, nil
	}
	// Return as many objects as were successfully added.
	n := len(m.addedData)
	if n > len(m.objects) {
		n = len(m.objects)
	}
	return m.objects[:n], nil
}

func (m *mockPackedUpload) Close() error {
	m.closed = true
	return nil
}

// mockSDK is a mock SDK for testing.
type mockSDK struct {
	upload            *mockPackedUpload
	uploadErr         error
	pinErr            error
	deleteErr         error
	pinnedObjs        []sdk.Object
	deletedObjs       []types.Hash256
	objectEvents      []sdk.ObjectEvent
	objectEventsErr   error
	objectEventsCalls int
}

func (m *mockSDK) UploadPacked(_ context.Context, _ ...sdk.UploadOption) (PackedUploader, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	return m.upload, nil
}

func (m *mockSDK) Download(_ context.Context, _ sdk.Object, _ ...sdk.DownloadOption) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSDK) PinObject(_ context.Context, obj sdk.Object) error {
	m.pinnedObjs = append(m.pinnedObjs, obj)
	return m.pinErr
}

func (m *mockSDK) DeleteObject(_ context.Context, key [32]byte) error {
	m.deletedObjs = append(m.deletedObjs, key)
	return m.deleteErr
}

func (m *mockSDK) SealObject(_ sdk.Object) sdk.SealedObject {
	return sdk.SealedObject{}
}

func (m *mockSDK) UnsealObject(_ sdk.SealedObject) (sdk.Object, error) {
	return sdk.Object{}, nil
}

func (m *mockSDK) ObjectEvents(_ context.Context, _ slabs.Cursor, _ int) ([]sdk.ObjectEvent, error) {
	m.objectEventsCalls++
	if m.objectEventsErr != nil {
		return nil, m.objectEventsErr
	}
	// Return events only on the first call so the loop terminates.
	if m.objectEventsCalls == 1 {
		return m.objectEvents, nil
	}
	return nil, nil
}

// --- Tests ---

func TestGroupStagedObjects_Empty(t *testing.T) {
	result := groupStagedObjects(nil, 1024, DefaultMaxGroupSize)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestGroupStagedObjects_SingleObject(t *testing.T) {
	objs := []models.RenterObject{{Size: 100}}
	result := groupStagedObjects(objs, 1024, DefaultMaxGroupSize)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if len(result[0]) != 1 {
		t.Errorf("expected 1 object in group, got %d", len(result[0]))
	}
}

func TestGroupStagedObjects_MultipleUnderSlab(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 100},
		{Model: gorm.Model{ID: 2}, Size: 200},
		{Model: gorm.Model{ID: 3}, Size: 300},
	}
	result := groupStagedObjects(objs, 1024, DefaultMaxGroupSize)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if len(result[0]) != 3 {
		t.Errorf("expected 3 objects in group, got %d", len(result[0]))
	}
}

func TestGroupStagedObjects_OverflowStartsNewGroup(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 600},
		{Model: gorm.Model{ID: 2}, Size: 600},
		{Model: gorm.Model{ID: 3}, Size: 100},
	}
	// Slab size 1024: first group gets obj1 (600), obj2 would push to 1200 > 1024.
	// So obj1 alone in group1, obj2+obj3 in group2.
	result := groupStagedObjects(objs, 1024, DefaultMaxGroupSize)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}
	if len(result[0]) != 1 {
		t.Errorf("expected 1 object in group 1, got %d", len(result[0]))
	}
	if len(result[1]) != 2 {
		t.Errorf("expected 2 objects in group 2, got %d", len(result[1]))
	}
}

func TestGroupStagedObjects_ExactFit(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 512},
		{Model: gorm.Model{ID: 2}, Size: 512},
	}
	// 512+512=1024, which is not > 1024, so both go in one group.
	result := groupStagedObjects(objs, 1024, DefaultMaxGroupSize)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if len(result[0]) != 2 {
		t.Errorf("expected 2 objects in group, got %d", len(result[0]))
	}
}

func TestGroupStagedObjects_LargerThanSlab(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 2048},
		{Model: gorm.Model{ID: 2}, Size: 100},
	}
	// First object exceeds slab size but still gets its own group.
	result := groupStagedObjects(objs, 1024, DefaultMaxGroupSize)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}
}

func TestGroupStagedObjects_MaxGroupSizeCap(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 400},
		{Model: gorm.Model{ID: 2}, Size: 400},
		{Model: gorm.Model{ID: 3}, Size: 400},
	}
	// slabSize=1024 would allow all 3 (1200 > 1024 splits after 2nd),
	// but maxGroupSize=500 forces a split after the first object.
	result := groupStagedObjects(objs, 1024, 500)
	if len(result) != 3 {
		t.Fatalf("expected 3 groups (maxGroupSize=500), got %d", len(result))
	}
	if len(result[0]) != 1 || len(result[1]) != 1 || len(result[2]) != 1 {
		t.Errorf("expected 1 object per group, got %d %d %d",
			len(result[0]), len(result[1]), len(result[2]))
	}
}

func TestGroupStagedObjects_MaxGroupSizeLargerThanSlab(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 600},
		{Model: gorm.Model{ID: 2}, Size: 600},
	}
	// maxGroupSize=2048 is larger than slabSize=1024, so slab boundary
	// governs the split.
	result := groupStagedObjects(objs, 1024, 2048)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups (slab boundary governs), got %d", len(result))
	}
}

func TestGroupStagedObjects_SingleObjectExceedsMaxGroupSize(t *testing.T) {
	objs := []models.RenterObject{
		{Model: gorm.Model{ID: 1}, Size: 2048},
	}
	// Object exceeds both slabSize and maxGroupSize, but still gets its own group.
	result := groupStagedObjects(objs, 1024, 512)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if len(result[0]) != 1 {
		t.Errorf("expected 1 object in group, got %d", len(result[0]))
	}
}

// errReader delivers data then returns err, simulating a partial read failure.
// Mirrors the siastorage SDK's errReader test pattern.
type errReader struct {
	data []byte
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func TestMockPackedUpload_AddSuccess(t *testing.T) {
	ctx := context.Background()
	upload := newMockPackedUpload(nil, -1) // no error

	data := []byte("hello")
	n, err := upload.Add(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("Add returned %d bytes, want %d", n, len(data))
	}
}

func TestMockPackedUpload_AddErrorOnIndex(t *testing.T) {
	ctx := context.Background()
	upload := newMockPackedUpload(nil, 1) // error on 2nd add (index 1)

	// First add succeeds
	_, err := upload.Add(ctx, bytes.NewReader([]byte("first")))
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}

	// Second add fails (simulates dead padding)
	_, err = upload.Add(ctx, bytes.NewReader([]byte("second")))
	if err == nil {
		t.Fatal("expected error on second Add, got nil")
	}

	// Third add should still work (new sub-group scenario)
	_, err = upload.Add(ctx, bytes.NewReader([]byte("third")))
	if err != nil {
		t.Fatalf("third Add failed: %v", err)
	}
}

func TestMockPackedUpload_FinalizeReturnsAddedCount(t *testing.T) {
	ctx := context.Background()

	// We need objects for Finalize to return. Create a mock with addErrOnIdx=-1
	// and override finalizeVals to return mock objects.
	upload := newMockPackedUpload(nil, -1)
	upload.Add(ctx, bytes.NewReader([]byte("a")))
	upload.Add(ctx, bytes.NewReader([]byte("b")))

	// Without finalizeVals, Finalize returns 0 objects (since m.objects is nil)
	objects, err := upload.Finalize(ctx)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}
	// mockPackedUpload returns m.objects[:n] where n = len(addedData)
	// Since m.objects is nil, n is clamped to 0
	if len(objects) != 0 {
		t.Errorf("expected 0 objects (no finalizeVals set), got %d", len(objects))
	}
}

func TestMockPackedUpload_ClosePreventsAdd(t *testing.T) {
	ctx := context.Background()
	upload := newMockPackedUpload(nil, -1)

	upload.Close()

	_, err := upload.Add(ctx, bytes.NewReader([]byte("data")))
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestMockSDK_TracksPinAndDelete(t *testing.T) {
	ctx := context.Background()
	mSDK := &mockSDK{}

	// PinObject should track the call
	obj := sdk.Object{} // can't easily create real sdk.Object, but mock doesn't validate
	_ = mSDK.PinObject(ctx, obj)
	if len(mSDK.pinnedObjs) != 1 {
		t.Errorf("expected 1 pinned object, got %d", len(mSDK.pinnedObjs))
	}

	// DeleteObject should track the call
	var key [32]byte
	_ = mSDK.DeleteObject(ctx, key)
	if len(mSDK.deletedObjs) != 1 {
		t.Errorf("expected 1 deleted object, got %d", len(mSDK.deletedObjs))
	}
}
