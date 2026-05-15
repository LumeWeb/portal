package service_tests

import (
	"context"
	"testing"
	"time"

	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/datatypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
)

func TestContentScannerService_ScanContent(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// Create a mock StorageHash
		mockStorageHash := mocks.NewMockStorageHash(tb)
		mockStorageHash.EXPECT().Multihash().Return([]byte("test_hash")).Maybe()
		mockStorageHash.EXPECT().String().Return("test_hash").Maybe()
		mockStorageHash.EXPECT().String().Return("test_hash").Maybe()
		mockStorageHash.EXPECT().String().Return("test_hash").Maybe()
		mockStorageHash.EXPECT().String().Return("test_hash").Maybe()

		// Test with no scanners registered
		results, err := contentScannerService.ScanContent(context.Background(), mockStorageHash)
		assert.NoError(tb, err)
		assert.Empty(tb, results)

		// Create a mock ContentScanner
		mockScanner := new(mocks.MockContentScanner)
		mockScanner.EXPECT().ID().Return("test_scanner").Maybe()
		mockScanner.EXPECT().Name().Return("Test Scanner").Maybe()
		mockScanner.EXPECT().Priority().Return(1).Maybe()
		mockScanner.EXPECT().ScanContent(mock.Anything, mockStorageHash).Return(&core.ScanResult{
			ScannerID: "test_scanner",
			Passed:    true,
			Reason:    "",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"test": "value"},
		}, nil).Maybe()

		// Register the mock scanner
		err = contentScannerService.RegisterScanner(mockScanner)
		require.NoError(tb, err)

		// Test with a scanner registered
		results, err = contentScannerService.ScanContent(context.Background(), mockStorageHash)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, results)
		assert.Equal(tb, "test_scanner", results[0].ScannerID)
		assert.True(tb, results[0].Passed)

		// Test GetScanResults
		scanResults, err := contentScannerService.GetScanResults(context.Background(), mockStorageHash)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, scanResults)
		assert.Equal(tb, "test_scanner", scanResults[0].ScannerID)
		assert.True(tb, scanResults[0].Passed)

	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}

func TestContentScannerService_RegisterScanner(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// Create a mock ContentScanner
		mockScanner1 := new(mocks.MockContentScanner)
		mockScanner1.EXPECT().ID().Return("test_scanner1").Maybe()
		mockScanner1.EXPECT().Name().Return("Test Scanner 1").Maybe()
		mockScanner1.EXPECT().Priority().Return(1).Maybe()

		mockScanner2 := new(mocks.MockContentScanner)
		mockScanner2.EXPECT().ID().Return("test_scanner2").Maybe()
		mockScanner2.EXPECT().Name().Return("Test Scanner 2").Maybe()
		mockScanner2.EXPECT().Priority().Return(2).Maybe()

		// Register the mock scanners
		err := contentScannerService.RegisterScanner(mockScanner1)
		require.NoError(tb, err)
		err = contentScannerService.RegisterScanner(mockScanner2)
		require.NoError(tb, err)

		// Check if scanners are registered and sorted by priority
		scanners := contentScannerService.RegisteredScanners()
		assert.Len(tb, scanners, 2)
		assert.Equal(tb, "test_scanner2", scanners[0].ID()) // Higher priority scanner should be first
		assert.Equal(tb, "test_scanner1", scanners[1].ID())

		// Test registering a scanner with a duplicate ID
		mockScanner3 := new(mocks.MockContentScanner)
		mockScanner3.EXPECT().ID().Return("test_scanner1").Maybe() // Duplicate ID
		err = contentScannerService.RegisterScanner(mockScanner3)
		assert.Error(tb, err)

		// Test registering a nil scanner
		err = contentScannerService.RegisterScanner(nil)
		assert.Error(tb, err)

	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}

func TestContentScannerService_GetScanResults(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// Create a mock StorageHash
		mockStorageHash := mocks.NewMockStorageHash(tb)
		mockStorageHash.EXPECT().Multihash().Return([]byte("test_hash")).Maybe()

		// Create a mock ContentScanner
		mockScanner := new(mocks.MockContentScanner)
		mockScanner.EXPECT().ID().Return("test_scanner").Maybe()
		mockScanner.EXPECT().Name().Return("Test Scanner").Maybe()
		mockScanner.EXPECT().Priority().Return(1).Maybe()
		mockStorageHash.EXPECT().String().Return("test_hash").Maybe()
		mockScanner.EXPECT().ScanContent(mock.Anything, mockStorageHash).Return(&core.ScanResult{
			ScannerID: "test_scanner",
			Passed:    true,
			Reason:    "",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"test": "value"},
		}, nil).Maybe()

		// Register the mock scanner
		err := contentScannerService.RegisterScanner(mockScanner)
		require.NoError(tb, err)

		// Scan content to store results
		_, err = contentScannerService.ScanContent(context.Background(), mockStorageHash)
		require.NoError(tb, err)

		// Retrieve scan results
		scanResults, err := contentScannerService.GetScanResults(context.Background(), mockStorageHash)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, scanResults)
		assert.Equal(tb, "test_scanner", scanResults[0].ScannerID)
		assert.True(tb, scanResults[0].Passed)
	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}

func TestContentScannerService_GetScanResultById(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// Create a test record directly in the database
		testHash := []byte("test_hash")
		testResult := &models.ScanResult{
			Hash:      testHash,
			ScannerID: "test_scanner",
			Passed:    true,
			Reason:    "test reason",
			Metadata:  datatypes.JSON(`{"test":"value"}`),
		}
		err := ctx.DB().Create(testResult).Error
		require.NoError(tb, err)

		// Retrieve scan result by ID
		scanResult, err := contentScannerService.GetScanResultById(context.Background(), uint(testResult.ID))
		assert.NoError(tb, err)
		assert.NotNil(tb, scanResult)
		assert.Equal(tb, "test_scanner", scanResult.ScannerID)
		assert.True(tb, scanResult.Passed)
		assert.Equal(tb, "test reason", scanResult.Reason)
	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}

func TestContentScannerService_RegisteredScanners(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		contentScannerService := core.GetService[core.ContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
		require.NotNil(tb, contentScannerService)

		// Create mock ContentScanners
		mockScanner1 := new(mocks.MockContentScanner)
		mockScanner1.EXPECT().ID().Return("test_scanner1").Maybe()
		mockScanner1.EXPECT().Name().Return("Test Scanner 1").Maybe()
		mockScanner1.EXPECT().Priority().Return(1).Maybe()

		mockScanner2 := new(mocks.MockContentScanner)
		mockScanner2.EXPECT().ID().Return("test_scanner2").Maybe()
		mockScanner2.EXPECT().Name().Return("Test Scanner 2").Maybe()
		mockScanner2.EXPECT().Priority().Return(2).Maybe()

		// Register the mock scanners
		err := contentScannerService.RegisterScanner(mockScanner1)
		require.NoError(tb, err)
		err = contentScannerService.RegisterScanner(mockScanner2)
		require.NoError(tb, err)

		// Retrieve registered scanners
		scanners := contentScannerService.RegisteredScanners()
		assert.Len(tb, scanners, 2)
		assert.Equal(tb, "test_scanner2", scanners[0].ID()) // Higher priority scanner should be first
		assert.Equal(tb, "test_scanner1", scanners[1].ID())
	}, coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService))
}
