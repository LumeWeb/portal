package core_tests

import (
	"bytes"
	"errors"
	"hash"
	"io"
	"testing"

	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

type failingSeeker struct {
	buf *bytes.Buffer
}

func (f *failingSeeker) Read(p []byte) (n int, err error) {
	return f.buf.Read(p)
}

func (f *failingSeeker) Seek(offset int64, whence int) (int64, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestNewMultiHasher_CreateMode(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgo1 := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}      // SHA2-256
	mockAlgo2 := core.HashAlgorithm{Type: 0xb220, Name: "BLAKE2b-256", Priority: 90, Protocol: "test"} // BLAKE2b-256

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	mockHash1 := mocks.NewMockTestingHashWithProof(t)
	mockHash1.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockHash1.On("Sum", mock.Anything).Return([]byte("sum1")).Maybe()
	mockHash1.On("GetProof").Return([]byte("proof1")).Maybe()
	mockHash1.On("Reset").Return().Maybe()
	mockHash1.On("Size").Return(32).Maybe()
	mockHash1.On("BlockSize").Return(64).Maybe()

	mockHash2 := mocks.NewMockTestingHashWithProof(t)
	mockHash2.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockHash2.On("Sum", mock.Anything).Return([]byte("sum2")).Maybe()
	mockHash2.On("GetProof").Return([]byte("proof2")).Maybe()
	mockHash2.On("Reset").Return().Maybe()
	mockHash2.On("Size").Return(32).Maybe()
	mockHash2.On("BlockSize").Return(64).Maybe()

	mockFactory := mocks.NewMockHashFactory(t)
	mockFactory.On("GetHasher", mockAlgo1.Type).Return(mockHash1, nil).Once()
	mockFactory.On("GetHasher", mockAlgo2.Type).Return(mockHash2, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	// Check that hashers were created for registered algorithms
	sums := hasher.Sums()
	assert.Len(t, sums, 2)

	foundAlgo1 := false
	foundAlgo2 := false
	for _, sum := range sums {
		if sum.Algorithm.Type == mockAlgo1.Type {
			foundAlgo1 = true
		}
		if sum.Algorithm.Type == mockAlgo2.Type {
			foundAlgo2 = true
		}
	}
	assert.True(t, foundAlgo1, "Should have a result for SHA-256")
	assert.True(t, foundAlgo2, "Should have a result for BLAKE2b-256")

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestNewMultiHasher_VerifyMode_StandardHasher(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a standard hash algorithm (doesn't implement proof interfaces)
	mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"} // SHA2-256
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	dummyHash, _ := mh.Encode([]byte("dummy data"), mh.SHA2_256)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, nil)

	verifyReq := &core.VerifyRequest{
		Algorithm: mockAlgo,
		Hash:      dummyStorageHash,
		Proof:     []byte("dummy proof"), // Proof is ignored by standard hashers
	}

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	// Use MockTestingHash which only implements hash.Hash
	mockHash := mocks.NewMockTestingHash(t)
	mockHash.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockHash.On("Sum", mock.Anything).Return([]byte("mocksum")).Maybe()
	mockHash.On("Reset").Return().Maybe()
	mockHash.On("Size").Return(32).Maybe()
	mockHash.On("BlockSize").Return(64).Maybe()

	decodedHash, err := mh.Decode(dummyStorageHash.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgo.Type, decodedHash.Length).Return(mockHash, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	sums := hasher.Sums()
	assert.Len(t, sums, 1)
	assert.Equal(t, mockAlgo.Type, sums[0].Algorithm.Type)
	assert.False(t, sums[0].Verified, "Standard hasher should not report verified")
	assert.NoError(t, sums[0].Error)                                  // No error expected during setup or verification (as it doesn't verify)
	assert.True(t, bytes.Equal([]byte("dummy proof"), sums[0].Proof)) // Proof should come from verify request
	assert.True(t, sums[0].Hash.ProofExists())                        // StorageHash should indicate proof exists if provided

	mockFactory.AssertExpectations(t)
	mockHash.AssertExpectations(t)
}

func TestNewMultiHasher_VerifyMode_WithProofProvider(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a mock hash algorithm that implements HashProofProvider
	mockAlgoType := uint64(0xff) // Custom mock type
	mockAlgoName := "MockProofProvider"
	mockAlgo := core.HashAlgorithm{Type: mockAlgoType, Name: mockAlgoName, Priority: 100, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash := mocks.NewMockTestingHashProofProvider(t) // Use a mock that implements HashProofProvider
	// Mock basic hash.Hash methods
	mockHash.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockHash.On("Sum", mock.Anything).Return([]byte("mocksum")).Maybe()
	mockHash.On("Reset").Return().Maybe()
	mockHash.On("Size").Return(32).Maybe()
	mockHash.On("BlockSize").Return(64).Maybe()
	// Mock SetProof
	mockHash.On("SetProof", []byte("expected proof")).Return(nil).Once()

	// Expect GetVariableHasher with the digest length from the dummy StorageHash
	dummyHash, _ := mh.Encode([]byte("dummy data"), mockAlgoType)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, []byte("expected proof"))
	decodedHash, err := mh.Decode(dummyStorageHash.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgoType, decodedHash.Length).Return(mockHash, nil).Once()

	verifyReq := &core.VerifyRequest{
		Algorithm: mockAlgo,
		Hash:      dummyStorageHash,
		Proof:     []byte("expected proof"),
	}

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)

	// Call Sums to trigger potential mock interactions (though SetProof happens in New)
	sums := hasher.Sums()
	assert.Len(t, sums, 1)
	assert.Equal(t, mockAlgoType, sums[0].Algorithm.Type)
	assert.True(t, bytes.Equal([]byte("expected proof"), sums[0].Proof))
	assert.True(t, sums[0].Hash.ProofExists())
	assert.False(t, sums[0].Verified) // Does not implement verifier
	assert.NoError(t, sums[0].Error)

	mockFactory.AssertExpectations(t)
	mockHash.AssertExpectations(t)
}

func TestNewMultiHasher_VerifyMode_WithSpecializedVerifier(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a mock hash algorithm with a specialized NewVerifier function
	mockAlgoType := uint64(0xfe) // Custom mock type
	mockAlgoName := "MockSpecializedVerifier"
	mockAlgo := core.HashAlgorithm{
		Type:     mockAlgoType,
		Name:     mockAlgoName,
		Priority: 100,
		Protocol: "test",
		NewVerifier: func(hash core.StorageHash, proof []byte) (hash.Hash, error) {
			mockHash := mocks.NewMockTestingHashProofVerifier(t)
			// Mock basic hash.Hash methods
			mockHash.On("Write", mock.Anything).Return(0, nil).Maybe()
			mockHash.On("Sum", mock.Anything).Return([]byte("mocksum")).Maybe()
			mockHash.On("Reset").Return().Maybe()
			mockHash.On("Size").Return(32).Maybe()
			mockHash.On("BlockSize").Return(64).Maybe()
			// Mock VerifyProof
			mockHash.On("VerifyProof").Return(true, nil).Once()
			return mockHash, nil
		},
	}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a dummy StorageHash for verification
	dummyHash, _ := mh.Encode([]byte("dummy data"), mockAlgoType)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, []byte("dummy proof"))

	verifyReq := &core.VerifyRequest{
		Algorithm: mockAlgo,
		Hash:      dummyStorageHash,
		Proof:     []byte("dummy proof"),
	}

	mockFactory := mocks.NewMockHashFactory(t)

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	sums := hasher.Sums()
	assert.Len(t, sums, 1)
	assert.Equal(t, mockAlgo.Type, sums[0].Algorithm.Type)
	assert.True(t, sums[0].Verified, "Specialized verifier should report verified")
	assert.NoError(t, sums[0].Error)
	assert.True(t, bytes.Equal([]byte("dummy proof"), sums[0].Proof)) // Proof should come from verify request
	assert.True(t, sums[0].Hash.ProofExists())                        // StorageHash should indicate proof exists if provided

	mockFactory.AssertExpectations(t) // Should have no calls for this algo type
}

func TestNewMultiHasher_VerifyMode_SpecializedVerifierError(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a mock hash algorithm with a specialized NewVerifier function that returns an error
	mockAlgoType := uint64(0xfd) // Custom mock type
	mockAlgoName := "MockSpecializedVerifierError"
	expectedErr := errors.New("verifier setup failed")
	mockAlgo := core.HashAlgorithm{
		Type:     mockAlgoType,
		Name:     mockAlgoName,
		Priority: 100,
		Protocol: "test",
		NewVerifier: func(hash core.StorageHash, proof []byte) (hash.Hash, error) {
			return nil, expectedErr
		},
	}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash := mocks.NewMockTestingHashProofProvider(t) // Use a provider mock for fallback check
	// Mock basic hash.Hash methods
	mockHash.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockHash.On("Sum", mock.Anything).Return([]byte("mocksum")).Maybe()
	mockHash.On("Reset").Return().Maybe()
	mockHash.On("Size").Return(32).Maybe()
	mockHash.On("BlockSize").Return(64).Maybe()
	mockHash.On("SetProof", []byte("dummy proof")).Return(nil).Once()

	// Expect GetVariableHasher with the digest length from the dummy StorageHash
	dummyHash, _ := mh.Encode([]byte("dummy data"), mockAlgoType)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, []byte("dummy proof"))
	decodedHash, err := mh.Decode(dummyStorageHash.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgoType, decodedHash.Length).Return(mockHash, nil).Once()

	verifyReq := &core.VerifyRequest{
		Algorithm: mockAlgo,
		Hash:      dummyStorageHash,
		Proof:     []byte("dummy proof"),
	}

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	// The fallback hasher's SetProof expectation should have been met

	sums := hasher.Sums()
	assert.Len(t, sums, 1)
	assert.Equal(t, mockAlgoType, sums[0].Algorithm.Type)
	assert.True(t, bytes.Equal([]byte("dummy proof"), sums[0].Proof))
	assert.True(t, sums[0].Hash.ProofExists())
	assert.False(t, sums[0].Verified) // Fallback hasher doesn't verify
	assert.NoError(t, sums[0].Error)  // No verification error from fallback

	mockFactory.AssertExpectations(t)
	mockHash.AssertExpectations(t)
}

func TestNewMultiHasher_VerifyMode_GetHasherError(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a mock hash algorithm
	mockAlgoType := uint64(0xfc) // Custom mock type
	mockAlgoName := "MockGetHasherError"
	mockAlgo := core.HashAlgorithm{Type: mockAlgoType, Name: mockAlgoName, Priority: 100, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a mock HashFactory that returns an error for this type
	mockFactory := mocks.NewMockHashFactory(t)
	dummyHash, _ := mh.Encode([]byte("dummy data"), mockAlgoType)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, nil)
	decodedHash, err := mh.Decode(dummyStorageHash.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgoType, decodedHash.Length).Return(nil, errors.New("get hasher failed")).Once()

	// Create a dummy StorageHash for verification
	verifyReq := &core.VerifyRequest{
		Algorithm: mockAlgo,
		Hash:      dummyStorageHash,
		Proof:     nil,
	}

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	sums := hasher.Sums()
	assert.Len(t, sums, 0) // No hasher should have been created for this algo

	mockFactory.AssertExpectations(t)
}

func TestMultiHasher_Write(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	mockAlgoType1 := uint64(0xfa)
	mockAlgoType2 := uint64(0xfb)
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}
	mockAlgo2 := core.HashAlgorithm{Type: mockAlgoType2, Name: "MockHash2", Priority: 90, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t) // Use a mock that implements Write
	mockHash2 := mocks.NewMockTestingHashWithProof(t) // Use a mock that implements Write

	// Expect GetHasher calls for create mode
	mockFactory.On("GetHasher", mockAlgoType1).Return(mockHash1, nil).Once()
	mockFactory.On("GetHasher", mockAlgoType2).Return(mockHash2, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	data := []byte("test data to hash")

	// Each mock should receive a copy of the data
	mockHash1.On("Write", data).Return(len(data), nil).Once()
	mockHash2.On("Write", data).Return(len(data), nil).Once()

	n, writeErr := hasher.Write(data)
	assert.NoError(t, writeErr)
	assert.Equal(t, len(data), n)

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestMultiHasher_Write_Error(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgoType1 := uint64(0xfa)
	mockAlgoType2 := uint64(0xfb)
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}
	mockAlgo2 := core.HashAlgorithm{Type: mockAlgoType2, Name: "MockHash2", Priority: 90, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t)
	mockHash2 := mocks.NewMockTestingHashWithProof(t)

	// Expect GetHasher calls for create mode
	mockFactory.On("GetHasher", mockAlgoType1).Return(mockHash1, nil).Once()
	mockFactory.On("GetHasher", mockAlgoType2).Return(mockHash2, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	data := []byte("test data to hash")
	expectedErr := errors.New("write error")

	// Mock one write to succeed and one to fail
	mockHash1.On("Write", data).Return(len(data), nil).Once()
	mockHash2.On("Write", data).Return(0, expectedErr).Once()

	n, writeErr := hasher.Write(data)
	assert.Error(t, writeErr)
	assert.Equal(t, expectedErr, writeErr)
	assert.Equal(t, 0, n) // Should return 0 on error

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestMultiHasher_Sums_CreateMode(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgoType1 := uint64(0xfa)
	mockAlgoType2 := uint64(0xfb)
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}
	mockAlgo2 := core.HashAlgorithm{Type: mockAlgoType2, Name: "MockHash2", Priority: 90, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t) // Implements HashProofGenerator
	mockHash2 := mocks.NewMockTestingHashWithProof(t) // Implements HashProofGenerator

	// Expect GetHasher calls for create mode
	mockFactory.On("GetHasher", mockAlgoType1).Return(mockHash1, nil).Once()
	mockFactory.On("GetHasher", mockAlgoType2).Return(mockHash2, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	expectedSum1 := []byte("sum1")
	expectedProof1 := []byte("proof1")
	expectedSum2 := []byte("sum2")
	expectedProof2 := []byte("proof2")

	mockHash1.On("Sum", mock.Anything).Return(expectedSum1).Once()
	mockHash1.On("GetProof").Return(expectedProof1).Once()
	mockHash2.On("Sum", mock.Anything).Return(expectedSum2).Once()
	mockHash2.On("GetProof").Return(expectedProof2).Once()

	sums := hasher.Sums()
	assert.Len(t, sums, 2)

	for _, result := range sums {
		decodedResultHash, err := mh.Decode(result.Hash.Multihash())
		require.NoError(t, err)

		if result.Algorithm.Type == mockAlgoType1 {
			assert.True(t, bytes.Equal(expectedSum1, decodedResultHash.Digest))
			assert.True(t, bytes.Equal(expectedProof1, result.Proof))
			assert.True(t, result.Hash.ProofExists()) // Should be true if proof is non-nil
			assert.False(t, result.Verified)          // Not in verify mode
			assert.NoError(t, result.Error)
		} else if result.Algorithm.Type == mockAlgoType2 {
			assert.True(t, bytes.Equal(expectedSum2, decodedResultHash.Digest))
			assert.True(t, bytes.Equal(expectedProof2, result.Proof))
			assert.True(t, result.Hash.ProofExists()) // Should be true if proof is non-nil
			assert.False(t, result.Verified)          // Not in verify mode
			assert.NoError(t, result.Error)
		} else {
			t.Errorf("Unexpected algorithm type in sums: %d", result.Algorithm.Type)
		}
	}

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestMultiHasher_Sums_VerifyMode(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgoType1 := uint64(0xfa) // Implements HashProofVerifier
	mockAlgoType2 := uint64(0xfb) // Does not implement HashProofVerifier
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}
	mockAlgo2 := core.HashAlgorithm{Type: mockAlgoType2, Name: "MockHash2", Priority: 90, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashProofVerifier(t) // Implements HashProofVerifier
	mockHash2 := mocks.NewMockTestingHashProofProvider(t) // Does NOT implement HashProofVerifier

	// Create dummy StorageHashes for verification
	dummyHash1, _ := mh.Encode([]byte("dummy data 1"), mockAlgoType1)
	dummyStorageHash1 := core.NewStorageHashFromMultihash(dummyHash1, 0, []byte("proof1"))
	dummyHash2, _ := mh.Encode([]byte("dummy data 2"), mockAlgoType2)
	dummyStorageHash2 := core.NewStorageHashFromMultihash(dummyHash2, 0, []byte("proof2"))

	// Expect GetVariableHasher calls with digest lengths
	decodedHash1, err := mh.Decode(dummyStorageHash1.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgoType1, decodedHash1.Length).Return(mockHash1, nil).Once()

	decodedHash2, err := mh.Decode(dummyStorageHash2.Multihash())
	require.NoError(t, err)
	mockFactory.On("GetVariableHasher", mockAlgoType2, decodedHash2.Length).Return(mockHash2, nil).Once()

	// Setup proof expectations - will be called once per hasher during Sums()
	mockHash1.On("SetProof", []byte("proof1")).Return(nil).Maybe()
	mockHash2.On("SetProof", []byte("proof2")).Return(nil).Maybe()

	verifyReq1 := &core.VerifyRequest{Algorithm: mockAlgo1, Hash: dummyStorageHash1, Proof: []byte("proof1")}
	verifyReq2 := &core.VerifyRequest{Algorithm: mockAlgo2, Hash: dummyStorageHash2, Proof: []byte("proof2")}

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq1, verifyReq2)
	defer hasher.Close()

	expectedSum1 := []byte("sum1")
	expectedSum2 := []byte("sum2")
	expectedVerifyErr := errors.New("verification failed")

	mockHash1.On("Sum", mock.Anything).Return(expectedSum1).Once()
	mockHash1.On("VerifyProof").Return(false, expectedVerifyErr).Once() // Simulate verification failure
	mockHash2.On("Sum", mock.Anything).Return(expectedSum2).Once()
	// mockHash2 does not have VerifyProof, so VerifyProof will not be called on it.

	sums := hasher.Sums()
	assert.Len(t, sums, 2)

	for _, result := range sums {
		decodedResultHash, err := mh.Decode(result.Hash.Multihash())
		require.NoError(t, err)

		if result.Algorithm.Type == mockAlgoType1 {
			assert.True(t, bytes.Equal(expectedSum1, decodedResultHash.Digest))
			assert.True(t, bytes.Equal([]byte("proof1"), result.Proof)) // Proof from VerifyRequest
			assert.True(t, result.Hash.ProofExists())                   // Should be true if proof is non-nil
			assert.False(t, result.Verified)                            // Verification failed
			assert.Equal(t, expectedVerifyErr, result.Error)
		} else if result.Algorithm.Type == mockAlgoType2 {
			assert.True(t, bytes.Equal(expectedSum2, decodedResultHash.Digest))
			assert.True(t, bytes.Equal([]byte("proof2"), result.Proof)) // Proof from VerifyRequest
			assert.True(t, result.Hash.ProofExists())                   // Should be true if proof is non-nil
			assert.False(t, result.Verified)                            // Does not implement verifier
			assert.NoError(t, result.Error)
		} else {
			t.Errorf("Unexpected algorithm type in sums: %d", result.Algorithm.Type)
		}
	}

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestMultiHasher_Close(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgo1 := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t)
	// Expect GetHasher for create mode
	mockFactory.On("GetHasher", mockAlgo1.Type).Return(mockHash1, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	assert.NotNil(t, hasher)

	hasher.Close()
	hasher.Close() // Calling Close again should be safe

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
}

func TestMultiHasher_Close_WithClosableSource(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithm
	mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a buffer wrapped in a NopCloser to implement io.ReadCloser
	testData := []byte("test data")
	source := bytes.NewBuffer(testData)
	closer := io.NopCloser(source)

	// Create hasher with closable source
	hasher, err := core.NewMultiHasherFromReader(closer)
	require.NoError(t, err)

	err = hasher.Close()
	assert.NoError(t, err)

	// Verify buffer is still intact
	assert.Equal(t, testData, source.Bytes())
}

func TestMultiHasher_Seek(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithm
	mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create mock seekable source
	testData := []byte("test data for seeking")
	source := bytes.NewReader(testData)

	// Create hasher with seekable source
	hasher, err := core.NewMultiHasherFromReader(source)
	assert.NoError(t, err)

	// Test seeking to start
	pos, err := hasher.Seek(0, io.SeekStart)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), pos)

	// Test seeking to end
	pos, err = hasher.Seek(0, io.SeekEnd)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(testData)), pos)

	// Test seeking to middle
	pos, err = hasher.Seek(5, io.SeekStart)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), pos)

	// Verify hashers were reset after seek
	for _, h := range hasher.GetHashes() {
		if h != nil {
			mockHash, ok := h.(*mocks.MockTestingHashWithProof)
			if ok {
				mockHash.AssertCalled(t, "Reset")
			}
		}
	}
}

func TestMultiHasher_Seek_ErrorCases(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	t.Run("NonSeekableSource", func(t *testing.T) {
		// Register mock hash algorithm
		mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
		registry := core.GetHashRegistry()
		err := registry.RegisterHashAlgorithm(mockAlgo)
		require.NoError(t, err)

		// Create hasher with non-seekable source
		source := bytes.NewBufferString("test data")
		hasher, err := core.NewMultiHasherFromReader(source)
		require.NoError(t, err)

		_, err = hasher.Seek(0, io.SeekStart)
		assert.Error(t, err)
		assert.Equal(t, errors.New("source does not support seeking"), err)
	})

	t.Run("SeekError", func(t *testing.T) {
		// Register mock hash algorithm
		mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
		registry := core.GetHashRegistry()
		err := registry.RegisterHashAlgorithm(mockAlgo)
		require.NoError(t, err)

		// Create buffer with failing seek
		failingSource := &failingSeeker{buf: bytes.NewBufferString("test data")}

		_, err = core.NewMultiHasherFromReader(failingSource)
		assert.Error(t, err)
		assert.Equal(t, err, io.ErrUnexpectedEOF)
	})
}

func TestMultiHasher_EmptyInput(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgoType1 := uint64(0xfa)
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t)

	// Expect GetHasher for create mode
	mockFactory.On("GetHasher", mockAlgoType1).Return(mockHash1, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	// Expect Write with empty byte slice
	mockHash1.On("Write", []byte{}).Return(0, nil).Once()
	n, writeErr := hasher.Write([]byte{})
	assert.NoError(t, writeErr)
	assert.Equal(t, 0, n)

	expectedSum1 := []byte("empty_sum")
	expectedProof1 := []byte("empty_proof")
	mockHash1.On("Sum", mock.Anything).Return(expectedSum1).Once()
	mockHash1.On("GetProof").Return(expectedProof1).Once()

	sums := hasher.Sums()
	assert.Len(t, sums, 1)

	decodedResultHash, err := mh.Decode(sums[0].Hash.Multihash())
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expectedSum1, decodedResultHash.Digest))

	assert.True(t, bytes.Equal(expectedProof1, sums[0].Proof))
	assert.True(t, sums[0].Hash.ProofExists()) // Should be true if proof is non-nil

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
}

func TestMultiHasher_MultipleWrites(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgoType1 := uint64(0xfa)
	mockAlgoType2 := uint64(0xfb)
	mockAlgo1 := core.HashAlgorithm{Type: mockAlgoType1, Name: "MockHash1", Priority: 100, Protocol: "test"}
	mockAlgo2 := core.HashAlgorithm{Type: mockAlgoType2, Name: "MockHash2", Priority: 90, Protocol: "test"}

	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)
	err = registry.RegisterHashAlgorithm(mockAlgo2)
	require.NoError(t, err)

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	mockHash1 := mocks.NewMockTestingHashWithProof(t)
	mockHash2 := mocks.NewMockTestingHashWithProof(t)

	// Expect GetHasher calls for create mode
	mockFactory.On("GetHasher", mockAlgoType1).Return(mockHash1, nil).Once()
	mockFactory.On("GetHasher", mockAlgoType2).Return(mockHash2, nil).Once()

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	data1 := []byte("part 1")
	data2 := []byte("part 2")

	// Due to the fix in Write, each hasher should receive the full data from each Write call.
	// The order of writes to the MultiHasher is sequential, but the writes to individual hashers
	// within a single MultiHasher.Write call are concurrent.
	// However, the test is written assuming sequential writes to the underlying hashers.
	// Let's adjust the mock expectations to match the sequential calls in the test.
	// A more robust test might involve capturing the data written to mocks and verifying the total.
	// For now, we'll assume the test intends to verify that each part is written.

	// Expect Write calls for data1 and data2 on each hasher
	mockHash1.On("Write", data1).Return(len(data1), nil).Once()
	mockHash1.On("Write", data2).Return(len(data2), nil).Once()
	mockHash2.On("Write", data1).Return(len(data1), nil).Once()
	mockHash2.On("Write", data2).Return(len(data2), nil).Once()

	n1, err1 := hasher.Write(data1)
	assert.NoError(t, err1)
	assert.Equal(t, len(data1), n1)

	n2, err2 := hasher.Write(data2)
	assert.NoError(t, err2)
	assert.Equal(t, len(data2), n2)

	expectedSum1 := []byte("combined_sum")
	expectedProof1 := []byte("combined_proof")
	expectedSum2 := []byte("combined_sum_2")     // Assuming different sum for the second hasher
	expectedProof2 := []byte("combined_proof_2") // Assuming different proof

	mockHash1.On("Sum", mock.Anything).Return(expectedSum1).Once()
	mockHash1.On("GetProof").Return(expectedProof1).Once()
	mockHash2.On("Sum", mock.Anything).Return(expectedSum2).Once()
	mockHash2.On("GetProof").Return(expectedProof2).Once()

	sums := hasher.Sums()
	assert.Len(t, sums, 2)

	for _, result := range sums {
		decodedResultHash, err := mh.Decode(result.Hash.Multihash())
		require.NoError(t, err)

		if result.Algorithm.Type == mockAlgoType1 {
			assert.True(t, bytes.Equal(expectedSum1, decodedResultHash.Digest))
			assert.True(t, bytes.Equal(expectedProof1, result.Proof))
			assert.True(t, result.Hash.ProofExists())
			assert.False(t, result.Verified)
			assert.NoError(t, result.Error)
		} else if result.Algorithm.Type == mockAlgoType2 {
			assert.True(t, bytes.Equal(expectedSum2, decodedResultHash.Digest))
			assert.True(t, bytes.Equal(expectedProof2, result.Proof))
			assert.True(t, result.Hash.ProofExists())
			assert.False(t, result.Verified)
			assert.NoError(t, result.Error)
		} else {
			t.Errorf("Unexpected algorithm type in sums: %d", result.Algorithm.Type)
		}
	}

	mockFactory.AssertExpectations(t)
	mockHash1.AssertExpectations(t)
	mockHash2.AssertExpectations(t)
}

func TestNewMultiHasherFromReader_BasicReader(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"} // SHA2-256
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a test reader
	testData := []byte("test data")
	reader := bytes.NewReader(testData)

	// Create hasher from reader
	hasher, err := core.NewMultiHasherFromReader(reader)
	require.NoError(t, err)
	defer func(hasher *core.MultiHasher) {
		err = hasher.Close()
		if err != nil {
			require.NoError(t, err)
		}
	}(hasher)

	// Read all data through the hasher
	readBuf := make([]byte, len(testData))
	n, err := hasher.Read(readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, readBuf)

	// Verify the hasher processed the data
	sums := hasher.Sums()
	assert.NotEmpty(t, sums)
}

func TestNewMultiHasherFromReader_SeekableReader(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register mock hash algorithms
	mockAlgo := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"} // SHA2-256
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo)
	require.NoError(t, err)

	// Create a test reader
	testData := []byte("test data")
	reader := bytes.NewReader(testData)

	// Create hasher from reader
	hasher, err := core.NewMultiHasherFromReader(reader)
	require.NoError(t, err)
	defer hasher.Close()

	// Verify seek position was reset to start
	pos, err := reader.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pos)

	// Read partial data
	partialBuf := make([]byte, 4)
	n, err := hasher.Read(partialBuf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, testData[:4], partialBuf)

	// Read remaining data
	remainingBuf := make([]byte, len(testData)-4)
	n, err = hasher.Read(remainingBuf)
	require.NoError(t, err)
	assert.Equal(t, len(testData)-4, n)
	assert.Equal(t, testData[4:], remainingBuf)

	// Verify all data was hashed
	sums := hasher.Sums()
	assert.NotEmpty(t, sums)
}

func TestNewMultiHasherFromReader_ErrorCases(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	t.Run("SeekError", func(t *testing.T) {
		// Create a reader that fails Seek
		failingReader := &failingSeeker{buf: bytes.NewBufferString("test data")}
		hasher, err := core.NewMultiHasherFromReader(failingReader)
		assert.Error(t, err)
		assert.Nil(t, hasher)
	})

	t.Run("ReadError", func(t *testing.T) {
		testData := []byte("test data")
		reader := bytes.NewReader(testData)

		hasher, err := core.NewMultiHasherFromReader(reader)
		require.NoError(t, err)
		defer hasher.Close()

		// Mock the Write to fail
		mockHash := mocks.NewMockTestingHashWithProof(t)
		mockHash.On("Write", mock.Anything).Return(0, errors.New("write error")).Once()
		hasher.SetHashes([]hash.Hash{mockHash})
		hasher.SetWriters([]io.Writer{mockHash})

		buf := make([]byte, 4)
		_, err = hasher.Read(buf)
		assert.Error(t, err)
		mockHash.AssertExpectations(t)
	})
}

func TestMultiHasher_NoRegisteredAlgorithms(t *testing.T) {
	core.ResetState() // Ensure no algorithms are registered
	defer core.ResetState()

	registry := core.GetHashRegistry()
	assert.Empty(t, registry.GetHashAlgorithms())

	// Create a mock HashFactory (should not be called)
	mockFactory := mocks.NewMockHashFactory(t)

	hasher := core.NewMultiHasherWithFactory(mockFactory)
	defer hasher.Close()

	assert.NotNil(t, hasher)

	data := []byte("some data")
	n, err := hasher.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)

	sums := hasher.Sums()
	assert.Empty(t, sums)

	mockFactory.AssertExpectations(t) // Should have no calls
}

func TestMultiHasher_VerifyMode_NoMatchingAlgorithm(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register one algorithm
	mockAlgo1 := core.HashAlgorithm{Type: 0x12, Name: "SHA-256", Priority: 100, Protocol: "test"}
	registry := core.GetHashRegistry()
	err := registry.RegisterHashAlgorithm(mockAlgo1)
	require.NoError(t, err)

	verifyAlgoType := uint64(0xff)
	verifyAlgo := core.HashAlgorithm{Type: verifyAlgoType, Name: "VerifyAlgo", Priority: 50, Protocol: "verify"}
	dummyHash, _ := mh.Encode([]byte("dummy data"), verifyAlgoType)
	dummyStorageHash := core.NewStorageHashFromMultihash(dummyHash, 0, nil)

	verifyReq := &core.VerifyRequest{
		Algorithm: verifyAlgo,
		Hash:      dummyStorageHash,
		Proof:     nil,
	}

	// Create a mock HashFactory
	mockFactory := mocks.NewMockHashFactory(t)
	// Expect GetHasher for the registered algorithm (since it's not in verifyReqs),
	// but no call for the verify algorithm type because it's not registered.
	// In verify mode, only hashers corresponding to verifyReqs are created.
	// Since mockAlgo1 is not in verifyReqs, no hasher should be created for it.
	// Since verifyAlgo is not registered, no hasher should be created for it either.
	// Therefore, the mock factory should not be called.

	hasher := core.NewMultiHasherWithFactory(mockFactory, verifyReq)
	defer hasher.Close()

	assert.NotNil(t, hasher)
	sums := hasher.Sums()
	assert.Len(t, sums, 0) // No hashers were created

	mockFactory.AssertExpectations(t) // Should have no calls
}
