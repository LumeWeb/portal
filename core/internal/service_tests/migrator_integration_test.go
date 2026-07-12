package service_tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdk "go.sia.tech/siastorage"
	"gorm.io/datatypes"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/migrator"
)

// --- Mock Lister/Downloader for the renterd client interfaces ---

type mockRenterdLister struct {
	objects map[string][]migrator.RenterdObjectMetadata
	err     error
}

func (m *mockRenterdLister) ListAllObjects(_ context.Context, bucket string) ([]migrator.RenterdObjectMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.objects[bucket], nil
}

type mockRenterdDownloader struct {
	data map[string][]byte
	err  error
	n    int
}

func (m *mockRenterdDownloader) DownloadObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	m.n++
	if m.err != nil {
		return nil, m.err
	}
	data, ok := m.data[bucket+"/"+key]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// --- Integration tests using coreTesting.RunTestCaseWithDB ---

// TestMigratorIntegration_HappyPath verifies that Migrate downloads objects
// from renterd, re-uploads them through RenterService.UploadObject (which
// pins, seals, and writes a DB row), and the resulting renter_objects
// table contains the expected rows.
func TestMigratorIntegration_HappyPath(t *testing.T) {
	slabSize := int64(1024)
	renter, mockSDK, _, opt := withRenterServiceAndMocks(slabSize)

	// Configure the mock SDK to produce a valid sealed object.
	mockSDK.SealObj = sdk.SealedObject{}

	objData := bytes.Repeat([]byte("x"), int(slabSize)+100)

	lister := &mockRenterdLister{
		objects: map[string][]migrator.RenterdObjectMetadata{
			"sia": {
				{Bucket: "sia", Key: "QmIntTest1", Size: int64(len(objData))},
			},
		},
	}
	downloader := &mockRenterdDownloader{
		data: map[string][]byte{
			"sia/QmIntTest1": objData,
		},
	}

	// Protocol that computes a real SHA-256 hash.
	proto := newMockStorageProtocol("sia", objData)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		m := &migrator.Migrator{
			Renter:     renter,
			Lister:     lister,
			Downloader: downloader,
			Logger:     ctx.Logger(),
		}

		stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err)
		assert.Equal(tb, 1, stats.Total)
		assert.Equal(tb, 1, stats.Migrated)
		assert.Equal(tb, 0, stats.Skipped)
		assert.Equal(tb, 0, stats.Errors)

		// Verify the object was pinned.
		assert.Len(tb, mockSDK.PinnedObjs, 1)

		// Verify the DB row.
		var row models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "QmIntTest1").First(&row).Error)
		assert.Equal(tb, "sia", row.Protocol)
		assert.Equal(tb, "QmIntTest1", row.ObjectKey)
		assert.Equal(tb, int64(len(objData)), row.Size)
		assert.Equal(tb, models.RenterObjectStatusUploaded, row.Status)
		assert.NotEmpty(tb, row.SiaObjectID)
		assert.NotEmpty(tb, row.SealedData)
	}, opt)
}

// TestMigratorIntegration_SkipsExisting verifies that when an object already
// exists in renter_objects, the migrator skips it without re-uploading.
func TestMigratorIntegration_SkipsExisting(t *testing.T) {
	slabSize := int64(1024)
	renter, mockSDK, _, opt := withRenterServiceAndMocks(slabSize)

	mockSDK.SealObj = sdk.SealedObject{}

	objData := bytes.Repeat([]byte("x"), int(slabSize)+100)

	lister := &mockRenterdLister{
		objects: map[string][]migrator.RenterdObjectMetadata{
			"sia": {
				{Bucket: "sia", Key: "QmExists1", Size: int64(len(objData))},
			},
		},
	}
	downloader := &mockRenterdDownloader{}

	proto := newMockStorageProtocol("sia", objData)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Pre-populate the DB with an existing object.
		existing := models.RenterObject{
			Protocol:    "sia",
			Bucket:      "sia",
			ObjectKey:   "QmExists1",
			SiaObjectID: "existing-id",
			SealedData:  datatypes.JSON([]byte("{}")),
			Size:        int64(len(objData)),
			Status:      models.RenterObjectStatusUploaded,
		}
		require.NoError(tb, ctx.DB().Create(&existing).Error)

		m := &migrator.Migrator{
			Renter:     renter,
			Lister:     lister,
			Downloader: downloader,
			Logger:     ctx.Logger(),
		}

		stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err)
		assert.Equal(tb, 1, stats.Total)
		assert.Equal(tb, 0, stats.Migrated)
		assert.Equal(tb, 1, stats.Skipped)
		assert.Equal(tb, 0, stats.Errors)

		// Verify no new pin happened.
		assert.Empty(tb, mockSDK.PinnedObjs)

		// Verify the original DB row is unchanged.
		var row models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "QmExists1").First(&row).Error)
		assert.Equal(tb, "existing-id", row.SiaObjectID)
	}, opt)
}

// TestMigratorIntegration_DryRun verifies that dry-run mode lists objects
// but does not upload or create DB rows.
func TestMigratorIntegration_DryRun(t *testing.T) {
	slabSize := int64(1024)
	renter, mockSDK, _, opt := withRenterServiceAndMocks(slabSize)

	objData := bytes.Repeat([]byte("x"), int(slabSize)+100)

	lister := &mockRenterdLister{
		objects: map[string][]migrator.RenterdObjectMetadata{
			"sia": {
				{Bucket: "sia", Key: "QmDry1", Size: int64(len(objData))},
			},
		},
	}
	downloader := &mockRenterdDownloader{}

	proto := newMockStorageProtocol("sia", objData)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		m := &migrator.Migrator{
			Renter:     renter,
			Lister:     lister,
			Downloader: downloader,
			Logger:     ctx.Logger(),
			DryRun:     true,
		}

		stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err)
		assert.Equal(tb, 1, stats.Total)
		assert.Equal(tb, 0, stats.Migrated)
		assert.Equal(tb, 0, stats.Skipped)
		assert.Equal(tb, 0, stats.Errors)

		// Verify no pin and no DB row.
		assert.Empty(tb, mockSDK.PinnedObjs)

		var count int64
		ctx.DB().Model(&models.RenterObject{}).Count(&count)
		assert.Equal(tb, int64(0), count)
	}, opt)
}

// TestMigratorIntegration_UploadError verifies that when UploadObject fails,
// the error is recorded and no DB row is created.
func TestMigratorIntegration_UploadError(t *testing.T) {
	slabSize := int64(1024)
	renter, mockSDK, _, opt := withRenterServiceAndMocks(slabSize)

	// Make UploadPacked fail.
	mockSDK.UploadErr = io.ErrUnexpectedEOF

	objData := bytes.Repeat([]byte("x"), int(slabSize)+100)

	lister := &mockRenterdLister{
		objects: map[string][]migrator.RenterdObjectMetadata{
			"sia": {
				{Bucket: "sia", Key: "QmFail1", Size: int64(len(objData))},
			},
		},
	}
	downloader := &mockRenterdDownloader{
		data: map[string][]byte{
			"sia/QmFail1": objData,
		},
	}

	proto := newMockStorageProtocol("sia", objData)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		m := &migrator.Migrator{
			Renter:     renter,
			Lister:     lister,
			Downloader: downloader,
			Logger:     ctx.Logger(),
		}

		stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err) // individual errors don't fail the whole migration
		assert.Equal(tb, 1, stats.Total)
		assert.Equal(tb, 0, stats.Migrated)
		assert.Equal(tb, 1, stats.Errors)

		// No DB row should exist.
		var count int64
		ctx.DB().Model(&models.RenterObject{}).Count(&count)
		assert.Equal(tb, int64(0), count)
	}, opt)
}

// TestMigratorIntegration_IdempotentRetry verifies that running Migrate twice
// does not create duplicate DB rows — the second run skips existing objects.
func TestMigratorIntegration_IdempotentRetry(t *testing.T) {
	slabSize := int64(1024)
	renter, mockSDK, _, opt := withRenterServiceAndMocks(slabSize)

	mockSDK.SealObj = sdk.SealedObject{}

	objData := bytes.Repeat([]byte("x"), int(slabSize)+100)

	lister := &mockRenterdLister{
		objects: map[string][]migrator.RenterdObjectMetadata{
			"sia": {
				{Bucket: "sia", Key: "QmIdem1", Size: int64(len(objData))},
			},
		},
	}
	downloader := &mockRenterdDownloader{
		data: map[string][]byte{
			"sia/QmIdem1": objData,
		},
	}

	proto := newMockStorageProtocol("sia", objData)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		m := &migrator.Migrator{
			Renter:     renter,
			Lister:     lister,
			Downloader: downloader,
			Logger:     ctx.Logger(),
		}

		// First migration — should upload.
		stats1, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err)
		assert.Equal(tb, 1, stats1.Migrated)
		assert.Equal(tb, 0, stats1.Skipped)

		// Second migration — should skip.
		stats2, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
		require.NoError(tb, err)
		assert.Equal(tb, 0, stats2.Migrated)
		assert.Equal(tb, 1, stats2.Skipped)

		// Only one DB row.
		var count int64
		ctx.DB().Model(&models.RenterObject{}).Count(&count)
		assert.Equal(tb, int64(1), count)
	}, opt)
}

// --- mockStorageProtocol ---

// mockStorageProtocol implements core.StorageProtocol for integration tests.
// It computes a real SHA-256 hash so that hash.Bytes() produces valid data.
type mockStorageProtocol struct {
	name string
	data []byte // data returned by Hash (used to compute SHA-256)
}

func newMockStorageProtocol(name string, data []byte) *mockStorageProtocol {
	return &mockStorageProtocol{name: name, data: data}
}

func (p *mockStorageProtocol) Name() string { return p.name }

func (p *mockStorageProtocol) EncodeFileName(_ core.StorageHash) string {
	// Return the encoded multihash as a hex string — matches what renterd
	// would have stored as the object key.
	h := sha256.Sum256(p.data)
	return hex.EncodeToString(h[:])
}

func (p *mockStorageProtocol) Hash(_ io.Reader, _ uint64) (core.StorageHash, error) {
	h := sha256.Sum256(p.data)
	return core.NewStorageHash(h[:], 0x12, 0, nil), nil
}

// Ensure mockStorageProtocol satisfies core.StorageProtocol.
var _ core.StorageProtocol = (*mockStorageProtocol)(nil)
