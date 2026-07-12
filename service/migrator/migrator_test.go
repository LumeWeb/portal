package migrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

// --- Hand-rolled mocks for simple interfaces (not in mockery config) ---

type mockLister struct {
	objects map[string][]RenterdObjectMetadata
	err     error
}

func (m *mockLister) ListAllObjects(_ context.Context, bucket string) ([]RenterdObjectMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.objects[bucket], nil
}

type mockDownloader struct {
	data      map[string][]byte
	err       error
	downloadN int
}

func (m *mockDownloader) DownloadObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	m.downloadN++
	if m.err != nil {
		return nil, m.err
	}
	data, ok := m.data[bucket+"/"+key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// testHash creates a valid StorageHash from raw data for testing.
func testHash() core.StorageHash {
	h := sha256.Sum256([]byte("test hash data"))
	return core.NewStorageHash(h[:], 0x12, 0, nil)
}

func testLogger() *core.Logger {
	return &core.Logger{Logger: zap.NewNop()}
}

// --- Tests ---

func TestMigrate_HappyPath(t *testing.T) {
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {
				{Bucket: "ipfs", Key: "QmTest1", Size: 5},
				{Bucket: "ipfs", Key: "QmTest2", Size: 3},
			},
		},
	}
	downloader := &mockDownloader{
		data: map[string][]byte{
			"ipfs/QmTest1": []byte("hello"),
			"ipfs/QmTest2": []byte("foo"),
		},
	}
	renter := mocks.NewMockRenterService(t)
	renter.EXPECT().UploadExists(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _, fileName string) (bool, *models.RenterObject, error) {
		return false, nil, nil
	})
	renter.EXPECT().UploadObject(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ io.Reader, _, _ string, _ []byte) error {
		return nil
	})

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()
	proto.EXPECT().Hash(mock.Anything, mock.Anything).RunAndReturn(func(_ io.Reader, _ uint64) (core.StorageHash, error) {
		return testHash(), nil
	})

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Total)
	assert.Equal(t, 2, stats.Migrated)
	assert.Equal(t, 0, stats.Skipped)
	assert.Equal(t, 0, stats.Errors)
}

func TestMigrate_DryRun(t *testing.T) {
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {
				{Bucket: "ipfs", Key: "QmTest1", Size: 5},
			},
		},
	}
	downloader := &mockDownloader{}
	renter := mocks.NewMockRenterService(t)

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
		DryRun:     true,
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, 0, stats.Migrated)
	assert.Equal(t, 0, stats.Skipped)
	assert.Equal(t, 0, stats.Errors)
}

func TestMigrate_SkipsExisting(t *testing.T) {
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {
				{Bucket: "ipfs", Key: "QmTest1", Size: 5},
			},
		},
	}
	downloader := &mockDownloader{
		data: map[string][]byte{
			"ipfs/QmTest1": []byte("hello"),
		},
	}
	renter := mocks.NewMockRenterService(t)
	renter.EXPECT().UploadExists(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _, _ string) (bool, *models.RenterObject, error) {
		return true, nil, nil
	})

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, 0, stats.Migrated)
	assert.Equal(t, 1, stats.Skipped)
	assert.Equal(t, 0, stats.Errors)
}

func TestMigrate_EmptyBucket(t *testing.T) {
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {},
		},
	}
	downloader := &mockDownloader{}
	renter := mocks.NewMockRenterService(t)

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0, stats.Migrated)
}

func TestMigrate_ListError(t *testing.T) {
	lister := &mockLister{
		err: errors.New("connection refused"),
	}
	downloader := &mockDownloader{}
	renter := mocks.NewMockRenterService(t)

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err) // list error doesn't abort the whole migration
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 1, stats.Errors)
}

func TestMigrate_ContextCancelled(t *testing.T) {
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {
				{Bucket: "ipfs", Key: "QmTest1", Size: 5},
			},
		},
	}
	downloader := &mockDownloader{
		data: map[string][]byte{
			"ipfs/QmTest1": []byte("hello"),
		},
	}
	renter := mocks.NewMockRenterService(t)

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()

	_, err := m.Migrate(ctx, []core.StorageProtocol{proto})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMigrationStats_String(t *testing.T) {
	s := MigrationStats{Total: 10, Migrated: 7, Skipped: 2, Errors: 1}
	assert.Equal(t, "total=10 migrated=7 skipped=2 errors=1", s.String())
}

// TestMigrate_StripsLeadingSlash verifies that object keys from renterd
// with a leading slash (e.g. /Qm...) are stripped before being used.
func TestMigrate_StripsLeadingSlash(t *testing.T) {
	var capturedKey string
	lister := &mockLister{
		objects: map[string][]RenterdObjectMetadata{
			"ipfs": {
				{Bucket: "ipfs", Key: "/QmTest1", Size: 5},
			},
		},
	}
	downloader := &mockDownloader{
		data: map[string][]byte{
			"ipfs/QmTest1": []byte("hello"),
		},
	}
	renter := mocks.NewMockRenterService(t)
	renter.EXPECT().UploadExists(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _, fileName string) (bool, *models.RenterObject, error) {
		capturedKey = fileName
		return false, nil, nil
	})
	renter.EXPECT().UploadObject(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ io.Reader, _, fileName string, _ []byte) error {
		capturedKey = fileName
		return nil
	})

	m := &Migrator{
		Renter:     renter,
		Lister:     lister,
		Downloader: downloader,
		Logger:     testLogger(),
	}

	proto := mocks.NewMockStorageProtocol(t)
	proto.EXPECT().Name().Return("ipfs").Maybe()
	proto.EXPECT().Hash(mock.Anything, mock.Anything).RunAndReturn(func(_ io.Reader, _ uint64) (core.StorageHash, error) {
		return testHash(), nil
	})

	stats, err := m.Migrate(context.Background(), []core.StorageProtocol{proto})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, 1, stats.Migrated)
	assert.Equal(t, "QmTest1", capturedKey)
}

// (helper removed — using mock.Anything for io.Reader args)
