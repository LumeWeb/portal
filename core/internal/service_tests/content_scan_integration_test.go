package service_tests

import (
	"context"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/service"

	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Define a simple ContentScanner implementation for testing
type testContentScanner struct {
	id       string
	name     string
	priority int
	result   bool
	reason   string
}

func (s *testContentScanner) ID() string {
	return s.id
}

func (s *testContentScanner) Name() string {
	return s.name
}

func (s *testContentScanner) Priority() int {
	return s.priority
}

func (s *testContentScanner) ScanContent(ctx context.Context, hash core.StorageHash) (*core.ScanResult, error) {
	return &core.ScanResult{
		ScannerID: s.id,
		Passed:    s.result,
		Reason:    s.reason,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"test": "value"},
	}, nil
}

func TestContentScannerService_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// 1. Create a StorageHash
		testHashBytes := []byte("test_hash")
		// Create a dummy multihash
		mh, err := multihash.Encode(testHashBytes, multihash.SHA2_256)
		require.NoError(tb, err)

		testStorageHash := &testStorageHash{mh: mh, hash: testHashBytes}

		// 2. Create ContentScanners
		scanner1 := &testContentScanner{
			id:       "test_scanner1",
			name:     "Test Scanner 1",
			priority: 1,
			result:   true,
			reason:   "",
		}
		scanner2 := &testContentScanner{
			id:       "test_scanner2",
			name:     "Test Scanner 2",
			priority: 2,
			result:   true,
			reason:   "",
		}

		// 3. Register the scanners
		err = contentScannerService.RegisterScanner(scanner1)
		require.NoError(tb, err)
		err = contentScannerService.RegisterScanner(scanner2)
		require.NoError(tb, err)

		// Verify that scanners are registered and sorted by priority
		scanners := contentScannerService.RegisteredScanners()
		assert.Len(tb, scanners, 2)
		assert.Equal(tb, "test_scanner2", scanners[0].ID()) // Higher priority scanner should be first
		assert.Equal(tb, "test_scanner1", scanners[1].ID())

		// 4. Test ScanContent
		results, err := contentScannerService.ScanContent(context.Background(), testStorageHash)
		assert.NoError(tb, err)
		assert.Len(tb, results, 2)
		// Check both scanner IDs are present (order depends on DB created_at resolution)
		resultIDs := make(map[string]bool)
		for _, r := range results {
			resultIDs[r.ScannerID] = true
			assert.True(tb, r.Passed)
		}
		assert.True(tb, resultIDs["test_scanner1"])
		assert.True(tb, resultIDs["test_scanner2"])

		// 5. Test GetScanResults
		scanResults, err := contentScannerService.GetScanResults(context.Background(), testStorageHash)
		assert.NoError(tb, err)
		assert.Len(tb, scanResults, 2)
		// Check both scanner IDs are present (order depends on DB created_at resolution)
		scanResultIDs := make(map[string]bool)
		for _, r := range scanResults {
			scanResultIDs[r.ScannerID] = true
			assert.True(tb, r.Passed)
		}
		assert.True(tb, scanResultIDs["test_scanner1"])
		assert.True(tb, scanResultIDs["test_scanner2"])

		// 6. Test GetScanResultById
		scanResult, err := contentScannerService.GetScanResultById(context.Background(), uint(scanResults[0].ID))
		assert.NoError(tb, err)
		assert.NotNil(tb, scanResult)
		assert.Contains(tb, []string{"test_scanner1", "test_scanner2"}, scanResult.ScannerID)
		assert.True(tb, scanResult.Passed)

		// 7. Register a scanner with a duplicate ID
		scanner3 := &testContentScanner{
			id:       "test_scanner1", // Duplicate ID
			name:     "Test Scanner 3",
			priority: 3,
			result:   true,
			reason:   "",
		}
		err = contentScannerService.RegisterScanner(scanner3)
		assert.Error(tb, err)

		// 8. Test registering a nil scanner
		err = contentScannerService.RegisterScanner(nil)
		assert.Error(tb, err)

		// 9. Test with no scanners registered
		// Clear registered scanners
		contentScannerService.(*service.ContentScannerServiceDefault).ClearScanners()
		results, err = contentScannerService.ScanContent(context.Background(), testStorageHash)
		assert.NoError(tb, err)
		assert.Empty(tb, results)

		// 10. Test storeScanResult via ScanContent
		tempScanner := &testContentScanner{
			id:       "temp_scanner",
			name:     "Temp Scanner",
			priority: 3,
			result:   true,
			reason:   "",
		}
		err = contentScannerService.RegisterScanner(tempScanner)
		require.NoError(tb, err)

		_, err = contentScannerService.ScanContent(context.Background(), testStorageHash)
		require.NoError(tb, err)

		storedResults, err := contentScannerService.GetScanResults(context.Background(), testStorageHash)
		require.NoError(tb, err)
		assert.NotEmpty(tb, storedResults)

		// Find the temp_scanner result
		var tempScannerResult *core.ScanResult
		for _, res := range storedResults {
			if res.ScannerID == tempScanner.ID() {
				tempScannerResult = res
				break
			}
		}

		assert.NotNil(tb, tempScannerResult, "temp_scanner result not found")
		assert.Equal(tb, tempScanner.ID(), tempScannerResult.ScannerID)
		assert.Equal(tb, tempScanner.result, tempScannerResult.Passed)
		assert.Equal(tb, tempScanner.reason, tempScannerResult.Reason)

	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}

// testStorageHash is a mock implementation of StorageHash for testing purposes.
type testStorageHash struct {
	mh   multihash.Multihash
	hash []byte
}

func (s *testStorageHash) Proof() []byte {
	return nil
}

func (s *testStorageHash) Multihash() multihash.Multihash {
	return s.mh
}

func (s *testStorageHash) ProofExists() bool {
	return false
}

func (s *testStorageHash) CIDType() uint64 {
	return 0
}

func (s *testStorageHash) Type() uint64 {
	return 0
}

func (s *testStorageHash) String() string {
	return string(s.hash)
}

func (s *testStorageHash) CIDString() string {
	if s.mh == nil {
		return ""
	}
	return cid.NewCidV0(s.mh).String()
}

func (s *testStorageHash) Bytes() []byte {
	return s.hash
}
