package service_tests

import (
	"context"
	"errors"
	"testing"

	"github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadService_SaveUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		testHashBytes := []byte("test_hash")
		mh, err := multihash.Sum(testHashBytes, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		// Create a test upload
		upload := &models.Upload{
			Hash:       mh,
			Protocol:   "test_protocol",
			UserID:     1,
			MimeType:   "test_mime",
			UploaderIP: "127.0.0.1",
			Size:       1024,
		}
		// Save the upload
		err = uploadService.SaveUpload(context.Background(), upload)
		require.NoError(tb, err)

		// Retrieve the upload and verify its values
		retrievedUpload, err := uploadService.GetUpload(context.Background(), &testStorageHash{hash: testHashBytes, mh: mh})
		require.NoError(tb, err)
		assert.Equal(tb, upload.Hash, retrievedUpload.Hash)
		assert.Equal(tb, upload.Protocol, retrievedUpload.Protocol)
		assert.Equal(tb, upload.UserID, retrievedUpload.UserID)
		assert.Equal(tb, upload.MimeType, retrievedUpload.MimeType)
		assert.Equal(tb, upload.UploaderIP, retrievedUpload.UploaderIP)
		assert.Equal(tb, upload.Size, retrievedUpload.Size)

		// Update the upload with new values
		upload.UserID = 2
		upload.MimeType = "new_mime"
		upload.UploaderIP = "127.0.0.2"
		upload.Size = 2048

		err = uploadService.SaveUpload(context.Background(), upload)
		require.NoError(tb, err)

		// Retrieve the updated upload and verify the new values
		updatedUpload, err := uploadService.GetUpload(context.Background(), &testStorageHash{hash: testHashBytes, mh: mh})
		require.NoError(tb, err)
		assert.Equal(tb, upload.UserID, updatedUpload.UserID)
		assert.Equal(tb, upload.MimeType, updatedUpload.MimeType)
		assert.Equal(tb, upload.UploaderIP, updatedUpload.UploaderIP)
		assert.Equal(tb, upload.Size, updatedUpload.Size)

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}

func TestUploadService_GetUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		testHashBytes := []byte("test_hash")
		mh, err := multihash.Sum(testHashBytes, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		// Create a test upload
		upload := &models.Upload{
			Hash:     mh,
			Protocol: "test_protocol",
		}
		err = uploadService.SaveUpload(context.Background(), upload)
		require.NoError(tb, err)

		// Retrieve the upload
		retrievedUpload, err := uploadService.GetUpload(context.Background(), &testStorageHash{hash: testHashBytes, mh: mh})
		require.NoError(tb, err)
		assert.Equal(tb, upload.Hash, retrievedUpload.Hash)
		assert.Equal(tb, upload.Protocol, retrievedUpload.Protocol)

		// Test with non-existent upload
		_, err = uploadService.GetUpload(context.Background(), &testStorageHash{mh: []byte("non_existent_hash")})
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

		// Test with nil hash
		_, err = uploadService.GetUpload(context.Background(), &testStorageHash{hash: nil})
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

		// Test with empty hash
		_, err = uploadService.GetUpload(context.Background(), &testStorageHash{hash: []byte{}})
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}

func TestUploadService_DeleteUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		// Create a test upload
		upload := &models.Upload{
			Hash:     []byte("test_hash"),
			Protocol: "test_protocol",
		}
		err := uploadService.SaveUpload(context.Background(), upload)
		require.NoError(tb, err)

		// Delete the upload
		err = uploadService.DeleteUpload(context.Background(), &testStorageHash{hash: []byte("test_hash")})
		require.NoError(tb, err)

		// Verify that the upload is deleted
		_, err = uploadService.GetUpload(context.Background(), &testStorageHash{hash: []byte("test_hash")})
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}

func TestUploadService_GetAllUploads(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		// Create test uploads
		upload1 := &models.Upload{
			Hash:     []byte("test_hash1"),
			Protocol: "test_protocol1",
		}
		upload2 := &models.Upload{
			Hash:     []byte("test_hash2"),
			Protocol: "test_protocol2",
		}
		err := uploadService.SaveUpload(context.Background(), upload1)
		require.NoError(tb, err)
		err = uploadService.SaveUpload(context.Background(), upload2)
		require.NoError(tb, err)

		// Get all uploads
		uploads, err := uploadService.GetAllUploads(context.Background())
		require.NoError(tb, err)
		assert.Len(tb, uploads, 2)

		// Verify the uploads
		assert.Equal(tb, upload1.Hash, uploads[0].Hash)
		assert.Equal(tb, upload1.Protocol, uploads[0].Protocol)
		assert.Equal(tb, upload2.Hash, uploads[1].Hash)
		assert.Equal(tb, upload2.Protocol, uploads[1].Protocol)

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}

func TestUploadService_GetUploadByID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		// Create a test upload
		upload := &models.Upload{
			Hash:     []byte("test_hash"),
			Protocol: "test_protocol",
		}
		err := uploadService.SaveUpload(context.Background(), upload)
		require.NoError(tb, err)

		// Retrieve the upload by ID
		retrievedUpload, err := uploadService.GetUploadByID(context.Background(), upload.ID)
		require.NoError(tb, err)
		assert.Equal(tb, upload.Hash, retrievedUpload.Hash)
		assert.Equal(tb, upload.Protocol, retrievedUpload.Protocol)

		// Test with non-existent upload ID
		_, err = uploadService.GetUploadByID(context.Background(), 999)
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}

func TestUploadService_GetUploadStats(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		uploadService := core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
		require.NotNil(tb, uploadService)

		// Empty DB — returns empty slice
		stats, err := uploadService.GetUploadStats(context.Background())
		require.NoError(tb, err)
		assert.Empty(tb, stats)

		// Seed uploads across two protocols
		uploads := []*models.Upload{
			{Hash: []byte("hash1"), Protocol: "ipfs", Size: 1024},
			{Hash: []byte("hash2"), Protocol: "ipfs", Size: 2048},
			{Hash: []byte("hash3"), Protocol: "sia", Size: 4096},
			{Hash: []byte("hash4"), Protocol: "sia", Size: 0}, // virtual object
		}
		for _, u := range uploads {
			require.NoError(tb, uploadService.SaveUpload(context.Background(), u))
		}

		stats, err = uploadService.GetUploadStats(context.Background())
		require.NoError(tb, err)
		require.Len(tb, stats, 2)

		// Build a map for deterministic lookup
		byProtocol := make(map[string]core.ProtocolUploadStat)
		for _, s := range stats {
			byProtocol[s.Protocol] = s
		}

		// IPFS: 2 uploads, 3072 bytes
		ipfsStat, ok := byProtocol["ipfs"]
		assert.True(tb, ok)
		assert.Equal(tb, uint64(2), ipfsStat.TotalUploads)
		assert.Equal(tb, uint64(3072), ipfsStat.TotalStorageBytes)

		// Sia: 2 uploads, 4096 bytes (Size=0 virtual object contributes 0)
		siaStat, ok := byProtocol["sia"]
		assert.True(tb, ok)
		assert.Equal(tb, uint64(2), siaStat.TotalUploads)
		assert.Equal(tb, uint64(4096), siaStat.TotalStorageBytes)

		// Soft-delete one IPFS upload and verify it's excluded
		require.NoError(tb, uploadService.DeleteUpload(context.Background(), &testStorageHash{hash: []byte("hash1")}))
		stats, err = uploadService.GetUploadStats(context.Background())
		require.NoError(tb, err)
		require.Len(tb, stats, 2)

		byProtocol = make(map[string]core.ProtocolUploadStat)
		for _, s := range stats {
			byProtocol[s.Protocol] = s
		}
		ipfsStat = byProtocol["ipfs"]
		assert.Equal(tb, uint64(1), ipfsStat.TotalUploads)
		assert.Equal(tb, uint64(2048), ipfsStat.TotalStorageBytes)

	}, coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService))
}
