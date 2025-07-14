package core_tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"testing"

	mb "github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

func TestNewStorageHash(t *testing.T) {
	hashBytes := []byte("test hash")
	hashType := uint64(mh.SHA2_256)
	cidType := uint64(0x01)
	proof := []byte("test proof")

	sh := core.NewStorageHash(hashBytes, hashType, cidType, proof)

	require.NotNil(t, sh)
	assert.True(t, bytes.Equal(proof, sh.Proof()))
	assert.True(t, sh.ProofExists())
	assert.Equal(t, cidType, sh.CIDType())
	assert.Equal(t, hashType, sh.Type())

	// Check the generated multihash
	expectedMultihash, err := mh.Encode(hashBytes, hashType)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expectedMultihash, sh.Multihash()))
	assert.Equal(t, mh.Multihash(expectedMultihash).B58String(), sh.String())
}

func TestNewStorageHash_NoProof(t *testing.T) {
	hashBytes := []byte("test hash")
	hashType := uint64(mh.SHA2_256)
	cidType := uint64(0x01)
	var proof []byte = nil

	sh := core.NewStorageHash(hashBytes, hashType, cidType, proof)

	require.NotNil(t, sh)
	assert.Nil(t, sh.Proof())
	assert.False(t, sh.ProofExists())
	assert.Equal(t, cidType, sh.CIDType())
	assert.Equal(t, hashType, sh.Type())

	expectedMultihash, err := mh.Encode(hashBytes, hashType)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expectedMultihash, sh.Multihash()))
	assert.Equal(t, mh.Multihash(expectedMultihash).B58String(), sh.String())
}

func TestNewStorageHashFromMultihash(t *testing.T) {
	hashBytes := []byte("test hash")
	hashType := uint64(mh.SHA2_256)
	cidType := uint64(0x01)
	proof := []byte("test proof")

	multihash, err := mh.Encode(hashBytes, hashType)
	require.NoError(t, err)

	sh := core.NewStorageHashFromMultihash(multihash, cidType, proof)

	require.NotNil(t, sh)
	assert.True(t, bytes.Equal(proof, sh.Proof()))
	assert.True(t, sh.ProofExists())
	assert.Equal(t, cidType, sh.CIDType())
	assert.Equal(t, hashType, sh.Type())
	assert.True(t, bytes.Equal(multihash, sh.Multihash()))
	assert.Equal(t, mh.Multihash(multihash).B58String(), sh.String())
}

func TestNewStorageHashFromMultihash_InvalidMultihash(t *testing.T) {
	invalidMultihash := mh.Multihash([]byte{0x01}) // Too short to be valid

	sh := core.NewStorageHashFromMultihash(invalidMultihash, 0x01, nil)
	assert.Nil(t, sh)
}

func TestNewStorageHashFromMultihashBytes(t *testing.T) {
	hashBytes := []byte("test hash")
	hashType := uint64(mh.SHA2_256)
	cidType := uint64(0x01)
	proof := []byte("test proof")

	multihashBytes, err := mh.Encode(hashBytes, hashType)
	require.NoError(t, err)

	sh := core.NewStorageHashFromMultihashBytes(multihashBytes, cidType, proof)

	require.NotNil(t, sh)
	assert.True(t, bytes.Equal(proof, sh.Proof()))
	assert.True(t, sh.ProofExists())
	assert.Equal(t, cidType, sh.CIDType())
	assert.Equal(t, hashType, sh.Type())
	assert.True(t, bytes.Equal(multihashBytes, sh.Multihash()))
	assert.Equal(t, mh.Multihash(multihashBytes).B58String(), sh.String()) // Corrected
}

func TestNewStorageHashFromMultihashBytes_InvalidBytes(t *testing.T) {
	invalidBytes := []byte{0x01} // Too short to be valid multihash

	sh := core.NewStorageHashFromMultihashBytes(invalidBytes, 0x01, nil)
	assert.Nil(t, sh)
}

func TestNewStorageHashFromRawMultihash(t *testing.T) {
	hashBytes := []byte("test hash")
	hashType := uint64(mh.SHA2_256)

	multihash, err := mh.Encode(hashBytes, hashType)
	require.NoError(t, err)

	sh := core.NewStorageHashFromRawMultihash(multihash)

	require.NotNil(t, sh)
	assert.Nil(t, sh.Proof())
	assert.False(t, sh.ProofExists())
	assert.Equal(t, uint64(0), sh.CIDType()) // Default CID type is 0
	assert.Equal(t, hashType, sh.Type())
	assert.True(t, bytes.Equal(multihash, sh.Multihash()))
	assert.Equal(t, mh.Multihash(multihash).B58String(), sh.String())
}

func TestParseStorageHash_Base58(t *testing.T) {
	inputData := []byte("test data for hashing") // Use different data to ensure a unique hash
	hasher := sha256.New()
	hasher.Write(inputData)
	hashDigest := hasher.Sum(nil)   // This will be 32 bytes for SHA2-256
	hashType := uint64(mh.SHA2_256) // 0x12
	multihash, err := mh.Encode(hashDigest, hashType)
	require.NoError(t, err)
	base58String := mh.Multihash(multihash).B58String()

	sh, err := core.ParseStorageHash(base58String)
	require.NoError(t, err)
	require.NotNil(t, sh)

	assert.True(t, bytes.Equal(multihash, sh.Multihash()))
	assert.Equal(t, hashType, sh.Type())
	assert.Equal(t, uint64(0x70), sh.CIDType())
	assert.Nil(t, sh.Proof())
	assert.False(t, sh.ProofExists())
	assert.Equal(t, base58String, sh.String())
}

func TestParseStorageHash_Hex(t *testing.T) {
	inputData := []byte("another test data for hashing") // Use different data
	hasher := sha256.New()
	hasher.Write(inputData)
	hashDigest := hasher.Sum(nil)   // This will be 32 bytes for SHA2-256
	hashType := uint64(mh.SHA2_256) // 0x12
	multihash, err := mh.Encode(hashDigest, hashType)
	require.NoError(t, err)

	// Test with multibase-encoded hex (should be recognized as CID)
	hexString, err := mb.Encode(mb.Base16, multihash)
	require.NoError(t, err)

	sh, err := core.ParseStorageHash(hexString)
	require.NoError(t, err)
	require.NotNil(t, sh)
	assert.True(t, bytes.Equal(multihash, sh.Multihash()))

	// Test with raw hex (should fail)
	rawHex := fmt.Sprintf("%x", multihash)
	sh, err = core.ParseStorageHash(rawHex)
	assert.Error(t, err)
	assert.Nil(t, sh)
	assert.ErrorAs(t, err, &core.ErrInvalidHashFormat, "error should be ErrInvalidHashFormat")
}

func TestParseStorageHash_Base64(t *testing.T) {
	tests := []struct {
		name     string
		encodeFn func([]byte) string
	}{
		{
			name: "Standard Base64",
			encodeFn: func(b []byte) string {
				return base64.StdEncoding.EncodeToString(b)
			},
		},
		{
			name: "Multibase Base64",
			encodeFn: func(b []byte) string {
				return "m" + base64.StdEncoding.EncodeToString(b)
			},
		},
		{
			name: "Multibase Base64URL",
			encodeFn: func(b []byte) string {
				return "u" + base64.URLEncoding.EncodeToString(b)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputData := []byte("test data " + tt.name)
			hasher := sha256.New()
			hasher.Write(inputData)
			hashDigest := hasher.Sum(nil)
			hashType := uint64(mh.SHA2_256)
			multihash, err := mh.Encode(hashDigest, hashType)
			require.NoError(t, err)

			encoded := tt.encodeFn(multihash)
			sh, err := core.ParseStorageHash(encoded)
			require.NoError(t, err)
			require.NotNil(t, sh)

			assert.True(t, bytes.Equal(multihash, sh.Multihash()))
			assert.Equal(t, hashType, sh.Type())
			assert.Equal(t, uint64(0x00), sh.CIDType())
			assert.Nil(t, sh.Proof())
			assert.False(t, sh.ProofExists())
			assert.Equal(t, mh.Multihash(multihash).B58String(), sh.String())
		})
	}
}

func TestParseStorageHash_Base32(t *testing.T) {
	tests := []struct {
		name     string
		encodeFn func([]byte) string
	}{
		{
			name: "Multibase Base32",
			encodeFn: func(b []byte) string {
				encoded, _ := mb.Encode(mb.Base32, b)
				return encoded
			},
		},
		{
			name: "Multibase Base32 Upper",
			encodeFn: func(b []byte) string {
				encoded, _ := mb.Encode(mb.Base32Upper, b)
				return encoded
			},
		},
		{
			name: "Multibase Base32 Pad",
			encodeFn: func(b []byte) string {
				encoded, _ := mb.Encode(mb.Base32pad, b)
				return encoded
			},
		},
		{
			name: "Multibase Base32 Pad Upper",
			encodeFn: func(b []byte) string {
				encoded, _ := mb.Encode(mb.Base32padUpper, b)
				return encoded
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputData := []byte("test data " + tt.name)
			hasher := sha256.New()
			hasher.Write(inputData)
			hashDigest := hasher.Sum(nil)
			hashType := uint64(mh.SHA2_256)
			multihash, err := mh.Encode(hashDigest, hashType)
			require.NoError(t, err)

			encoded := tt.encodeFn(multihash)
			sh, err := core.ParseStorageHash(encoded)
			require.NoError(t, err)
			require.NotNil(t, sh)
			assert.True(t, bytes.Equal(multihash, sh.Multihash()))
			assert.Equal(t, hashType, sh.Type())
			assert.Equal(t, uint64(0x70), sh.CIDType())
			assert.Nil(t, sh.Proof())
			assert.False(t, sh.ProofExists())
			assert.Equal(t, mh.Multihash(multihash).B58String(), sh.String())
		})
	}
}

func TestParseStorageHash_InvalidFormat(t *testing.T) {
	invalidString := "not a hash"

	sh, err := core.ParseStorageHash(invalidString)
	assert.Error(t, err)
	assert.Nil(t, sh)
	assert.Equal(t, core.ErrInvalidHashFormat, err)
}

func TestParseStorageHash_InvalidMultihashStructure(t *testing.T) {
	// Base58 string that decodes but isn't a valid multihash
	invalidBase58 := "12345" // Decodes to bytes, but not a valid multihash prefix

	sh, err := core.ParseStorageHash(invalidBase58)
	assert.Error(t, err)
	assert.Nil(t, sh)
	assert.Equal(t, err, core.ErrInvalidHashFormat, "error should indicate invalid format")

	// Hex string that decodes but isn't a valid multihash
	invalidHex := "aabbcc" // Decodes to bytes, but not a valid multihash prefix

	sh, err = core.ParseStorageHash(invalidHex)
	assert.Error(t, err)
	assert.Nil(t, sh)
	assert.Equal(t, err, core.ErrInvalidHashFormat, "error should indicate invalid format")

	// Base64 string that decodes but isn't a valid multihash
	invalidMultihashStructureBytes := []byte{0x01, 0x01} // Decodes, but not a valid multihash prefix
	invalidMultihashStructureString := base64.StdEncoding.EncodeToString(invalidMultihashStructureBytes)
	// Test Base64 parser with invalid multihash structure
	_, recognized, parseErr := (&core.CoreBase64Parser{}).TryParse(invalidMultihashStructureString)
	assert.True(t, recognized, "should recognize as Base64 format")
	assert.Error(t, parseErr, "should return error for invalid multihash structure")
	if parseErr != nil {
		assert.Contains(t, parseErr.Error(), "string is valid Base64 but content is not a valid multihash", "error should indicate invalid multihash")
	}

	// Test Base32 - expect it to fail with ErrInvalidHashFormat
	invalidMultihashStructureBytes = []byte{0x01, 0x01} // Decodes, but not a valid multihash prefix
	invalidMultihashStructureString = base32.StdEncoding.EncodeToString(invalidMultihashStructureBytes)
	sh, err = core.ParseStorageHash(invalidMultihashStructureString)
	assert.Error(t, err)
	assert.Nil(t, sh)
	assert.Equal(t, err, core.ErrInvalidHashFormat, "error should indicate invalid format")
}

func TestInferCIDTypeFromHashCode(t *testing.T) {
	tests := []struct {
		code      uint64
		digestLen int
		expected  uint64
		testName  string
	}{
		{mh.SHA2_256, 32, 0x00, "SHA2-256 32 bytes (CIDv0)"},
		{mh.SHA2_256, 16, 0x55, "SHA2-256 < 32 bytes (Raw)"},
		{mh.SHA2_256, 64, 0x01, "SHA2-256 > 32 bytes (CIDv1)"}, // Not standard, but should default to CIDv1
		{0xb220, 32, 0x01, "BLAKE2b-256 32 bytes (CIDv1)"},     // Using BLAKE2B-256 code from multihash constants
		{mh.IDENTITY, 10, 0x55, "IDENTITY (Raw)"},
		{0x1000, 20, 0x01, "Unknown code (CIDv1)"},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			actual := core.InferCIDTypeFromHashCode(tt.code, tt.digestLen)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// Test GetParsersFromRegistry (requires mocking the registry or ensuring it's empty/has known state)
// This test is more complex as it depends on the global registry state.
// For a true unit test, we might need to inject the registry or use a test-specific registry.
// Assuming GetProtocolList() returns an empty list for this test environment by default.
func TestGetParsersFromRegistry(t *testing.T) {
	// Ensure the global registry is in a known state (e.g., empty protocols)
	// This might require a core.ResetState() or similar if available and safe.
	// For now, we assume GetProtocolList() returns an empty slice in this test context.

	parsers := core.GetParsersFromRegistry() // Corrected: Call the exported function

	// Expect the core fallback parsers in the defined order
	expectedParserNames := []string{
		"CIDStorageHashParser",
		"MultihashStorageHashParser",
		"CoreBase64Multihash",
	}

	assert.Len(t, parsers, len(expectedParserNames))
	for i, parser := range parsers {
		assert.Equal(t, expectedParserNames[i], parser.ParserName())
	}

	// TODO: Add a test case that registers a mock protocol implementing StorageHashParser
	// to verify that protocol parsers are included and ordered correctly.
}

// Test ParseStorageHash using the registry (integration-like test)
func TestParseStorageHash_UsingRegistry(t *testing.T) {
	core.ResetState() // Ensure a clean registry
	defer core.ResetState()

	// Register a mock protocol parser
	mockProtocolName := "mock-protocol"
	mockProtocolParser := mocks.NewMockProtocolParser(t)
	core.RegisterProtocol(mockProtocolName, mockProtocolParser)

	// Create a hash string that the mock parser will handle
	mockHashString := "mock::somehash"
	expectedStorageHash := core.NewStorageHash([]byte("mockhashbytes"), 0x100, 0x200, nil)
	mockProtocolParser.On("TryParse", mockHashString).Return(expectedStorageHash, true, nil).Once()
	mockProtocolParser.On("Name").Return(mockProtocolName).Maybe()
	mockProtocolParser.On("ParserName").Return(mockProtocolName).Maybe()

	// Test parsing the mock protocol hash
	sh, err := core.ParseStorageHash(mockHashString)
	require.NoError(t, err)
	require.NotNil(t, sh)
	assert.True(t, bytes.Equal(expectedStorageHash.Multihash(), sh.Multihash())) // Compare underlying multihash
	assert.Equal(t, expectedStorageHash.Type(), sh.Type())
	assert.Equal(t, expectedStorageHash.CIDType(), sh.CIDType())

	// The mock parser should only be called once with the mock hash string
	mockProtocolParser.AssertExpectations(t)
}
