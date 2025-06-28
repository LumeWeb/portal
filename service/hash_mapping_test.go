package service

import (
	"bytes"
	"context"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"testing"
)

func TestHashMappingService_StoreGetMapping(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes := []byte("target_hash")

		sourceMH, err := mh.Encode(sourceHashBytes, mh.SHA2_256)
		require.NoError(tb, err)
		targetMH, err := mh.Encode(targetHashBytes, mh.SHA2_256)
		require.NoError(tb, err)

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Define test protocol and metadata
		protocol := "test_protocol"
		metadata := map[string]interface{}{"key": "value"}

		// Store the mapping
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash, protocol, metadata)
		require.NoError(tb, err)

		// Retrieve the mappings
		mappings, err := hashMappingService.GetMappings(context.Background(), sourceHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 1)
		assert.True(tb, bytes.Equal(targetMH, mappings[0].Multihash()))

		// Retrieve the reverse mappings
		reverseMappings, err := hashMappingService.GetReverseMappings(context.Background(), targetHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, reverseMappings, 1)
		assert.True(tb, bytes.Equal(sourceMH, reverseMappings[0].Multihash()))

		// Delete the mappings
		err = hashMappingService.DeleteMappings(context.Background(), sourceHash)
		require.NoError(tb, err)

		// Verify that the mappings are deleted
		mappings, err = hashMappingService.GetMappings(context.Background(), sourceHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 0)

		reverseMappings, err = hashMappingService.GetReverseMappings(context.Background(), targetHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, reverseMappings, 0)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_GetMappings_NoProtocol(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes1 := []byte("target_hash1")
		targetHashBytes2 := []byte("target_hash2")

		targetMH1, err := mh.Encode(targetHashBytes1, mh.SHA2_256)
		require.NoError(tb, err)
		targetMH2, err := mh.Encode(targetHashBytes2, mh.SHA2_256)
		require.NoError(tb, err)

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash1 := core.NewStorageHash(targetHashBytes1, mh.SHA2_256, 0, nil)
		targetHash2 := core.NewStorageHash(targetHashBytes2, mh.SHA2_256, 0, nil)

		// Define test protocols
		protocol1 := "test_protocol1"
		protocol2 := "test_protocol2"

		// Store the mappings with different protocols
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash1, protocol1, nil)
		require.NoError(tb, err)
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash2, protocol2, nil)
		require.NoError(tb, err)

		// Retrieve mappings without specifying protocol
		mappings, err := hashMappingService.GetMappings(context.Background(), sourceHash)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 2)

		// Check if both target hashes are present in the result
		var found1, found2 bool
		for _, mapping := range mappings {
			if bytes.Equal(mapping.Multihash(), targetMH1) {
				found1 = true
			}
			if bytes.Equal(mapping.Multihash(), targetMH2) {
				found2 = true
			}
		}
		assert.True(tb, found1, "targetHash1 should be present")
		assert.True(tb, found2, "targetHash2 should be present")

		// Clean up
		err = hashMappingService.DeleteMappings(context.Background(), sourceHash)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_GetReverseMappings_NoProtocol(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes1 := []byte("source_hash1")
		sourceHashBytes2 := []byte("source_hash2")
		targetHashBytes := []byte("target_hash")

		sourceMH1, err := mh.Encode(sourceHashBytes1, mh.SHA2_256)
		require.NoError(tb, err)
		sourceMH2, err := mh.Encode(sourceHashBytes2, mh.SHA2_256)
		require.NoError(tb, err)

		sourceHash1 := core.NewStorageHash(sourceHashBytes1, mh.SHA2_256, 0, nil)
		sourceHash2 := core.NewStorageHash(sourceHashBytes2, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Define test protocols
		protocol1 := "test_protocol1"
		protocol2 := "test_protocol2"

		// Store the mappings with different protocols
		err = hashMappingService.StoreMapping(context.Background(), sourceHash1, targetHash, protocol1, nil)
		require.NoError(tb, err)
		err = hashMappingService.StoreMapping(context.Background(), sourceHash2, targetHash, protocol2, nil)
		require.NoError(tb, err)

		// Retrieve reverse mappings without specifying protocol
		mappings, err := hashMappingService.GetReverseMappings(context.Background(), targetHash)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 2)

		// Check if both source hashes are present in the result
		var found1, found2 bool
		for _, mapping := range mappings {
			if bytes.Equal(mapping.Multihash(), sourceMH1) {
				found1 = true
			}
			if bytes.Equal(mapping.Multihash(), sourceMH2) {
				found2 = true
			}
		}
		assert.True(tb, found1, "sourceHash1 should be present")
		assert.True(tb, found2, "sourceHash2 should be present")

		// Clean up
		err = hashMappingService.DeleteMappings(context.Background(), targetHash)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_EmptyProtocol(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes1 := []byte("target_hash1")
		targetHashBytes2 := []byte("target_hash2")

		targetMH2, err := mh.Encode(targetHashBytes2, mh.SHA2_256)
		require.NoError(tb, err)

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash1 := core.NewStorageHash(targetHashBytes1, mh.SHA2_256, 0, nil)
		targetHash2 := core.NewStorageHash(targetHashBytes2, mh.SHA2_256, 0, nil)

		// Define test protocols
		protocol1 := "test_protocol1"

		// Store the mappings with different protocols
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash1, protocol1, nil)
		require.NoError(tb, err)
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash2, "", nil)
		require.NoError(tb, err)

		// Retrieve mappings with empty protocol (should only return mappings with empty protocol)
		mappings, err := hashMappingService.GetMappings(context.Background(), sourceHash, "")
		require.NoError(tb, err)
		assert.Len(tb, mappings, 1)

		// Verify the correct mapping was returned
		assert.True(tb, bytes.Equal(mappings[0].Multihash(), targetMH2), "should return mapping with empty protocol")

		// Clean up
		err = hashMappingService.DeleteMappings(context.Background(), sourceHash)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_InvalidHash(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes := []byte("target_hash")

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Define test protocol and metadata
		protocol := "test_protocol"
		metadata := map[string]interface{}{"key": "value"}

		// Store the mapping
		err := hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash, protocol, metadata)
		require.NoError(tb, err)

		// Retrieve the mappings
		invalidHash := core.NewStorageHash([]byte("invalid"), 12345, 0, nil)
		mappings, err := hashMappingService.GetMappings(context.Background(), invalidHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 0)

		// Retrieve the reverse mappings
		reverseMappings, err := hashMappingService.GetReverseMappings(context.Background(), invalidHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, reverseMappings, 0)

		// Clean up
		err = hashMappingService.DeleteMappings(context.Background(), sourceHash)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_MarshalError(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes := []byte("target_hash")

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Define test protocol and metadata
		protocol := "test_protocol"
		metadata := map[string]interface{}{
			"key": func() {}, // non-marshalable type
		}

		// Store the mapping
		err := hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash, protocol, metadata)
		require.Error(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_NilStorageHash(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Define test protocol and metadata
		protocol := "test_protocol"
		metadata := map[string]interface{}{"key": "value"}

		// Store the mapping with nil hashes
		err := hashMappingService.StoreMapping(context.Background(), nil, nil, protocol, metadata)
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "cannot be nil")

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes := []byte("target_hash")
		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Store the mapping with valid hashes
		err = hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash, protocol, metadata)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}

func TestHashMappingService_StoreAndRetrieveMappings(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		hashMappingService := core.GetService[core.HashMappingService](ctx, core.HASH_MAPPING_SERVICE)
		require.NotNil(tb, hashMappingService)

		// Create test StorageHashes
		sourceHashBytes := []byte("source_hash")
		targetHashBytes := []byte("target_hash")

		sourceHash := core.NewStorageHash(sourceHashBytes, mh.SHA2_256, 0, nil)
		targetHash := core.NewStorageHash(targetHashBytes, mh.SHA2_256, 0, nil)

		// Define test protocol and metadata
		protocol := "test_protocol"
		metadata := map[string]interface{}{"key": "value"}

		// Mock the DB to return an error during the transaction
		db := ctx.DB()
		require.NotNil(tb, db)

		// Store the mapping
		err := hashMappingService.StoreMapping(context.Background(), sourceHash, targetHash, protocol, metadata)
		require.NoError(tb, err)

		// Retrieve the mappings
		mappings, err := hashMappingService.GetMappings(context.Background(), sourceHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 1)

		// Mock the DB to return an error during the transaction
		//ctx.SetDB(db.Error)

		// Retrieve the mappings
		mappings, err = hashMappingService.GetMappings(context.Background(), sourceHash, protocol)
		require.NoError(tb, err)
		assert.Len(tb, mappings, 1)

		// Clean up
		err = hashMappingService.DeleteMappings(context.Background(), sourceHash)
		require.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, NewHashMappingService))
}
