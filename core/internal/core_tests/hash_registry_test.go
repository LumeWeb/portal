package core_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
)

func TestHashRegistry_RegisterHashAlgorithm(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	registry := core.GetHashRegistry()

	// Test successful registration
	err := registry.RegisterHashAlgorithm(core.HashAlgorithm{
		Type:     0x12, // SHA2-256
		Name:     "SHA-256",
		Priority: 100,
		Protocol: "test",
	})
	assert.NoError(t, err)

	// Test duplicate type (should be idempotent)
	err = registry.RegisterHashAlgorithm(core.HashAlgorithm{
		Type:     0x12, // Same type
		Name:     "SHA-256-dup",
		Priority: 50,
		Protocol: "test2",
	})
	assert.NoError(t, err)

	// Verify only one algorithm with this type exists
	algos := registry.GetHashAlgorithms()
	assert.Len(t, algos, 1)
	assert.Equal(t, "SHA-256", algos[0].Name) // Original name should be kept
	assert.Equal(t, 100, algos[0].Priority)   // Original priority should be kept
}

func TestHashRegistry_GetHashAlgorithms(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	registry := core.GetHashRegistry()

	// Register multiple algorithms with different priorities
	algorithms := []core.HashAlgorithm{
		{
			Type:     0x12, // SHA2-256
			Name:     "SHA-256",
			Priority: 100,
			Protocol: "test",
		},
		{
			Type:     0xb220, // BLAKE2b-256
			Name:     "BLAKE2b-256",
			Priority: 90,
			Protocol: "test",
		},
		{
			Type:     0x1b, // SHA3-256
			Name:     "SHA3-256",
			Priority: 110,
			Protocol: "test2",
		},
	}

	for _, algo := range algorithms {
		err := registry.RegisterHashAlgorithm(algo)
		assert.NoError(t, err)
	}

	// Get all algorithms
	retrieved := registry.GetHashAlgorithms()
	assert.Len(t, retrieved, 3)

	// Verify algorithms are sorted by priority (descending)
	assert.Equal(t, "SHA3-256", retrieved[0].Name)    // Highest priority (110)
	assert.Equal(t, "SHA-256", retrieved[1].Name)     // Next priority (100)
	assert.Equal(t, "BLAKE2b-256", retrieved[2].Name) // Lowest priority (90)
}

func TestHashRegistry_GetProtocolAlgorithms(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	registry := core.GetHashRegistry()

	// Register algorithms for different protocols
	algorithms := []core.HashAlgorithm{
		{
			Type:     0x12, // SHA2-256
			Name:     "SHA-256",
			Priority: 100,
			Protocol: "protocol1",
		},
		{
			Type:     0xb220, // BLAKE2b-256
			Name:     "BLAKE2b-256",
			Priority: 90,
			Protocol: "protocol1",
		},
		{
			Type:     0x1b, // SHA3-256
			Name:     "SHA3-256",
			Priority: 110,
			Protocol: "protocol2",
		},
	}

	for _, algo := range algorithms {
		err := registry.RegisterHashAlgorithm(algo)
		assert.NoError(t, err)
	}

	// Get algorithms for protocol1
	proto1Algos := registry.GetProtocolAlgorithms("protocol1")
	assert.Len(t, proto1Algos, 2)
	for _, algo := range proto1Algos {
		assert.Equal(t, "protocol1", algo.Protocol)
	}

	// Get algorithms for protocol2
	proto2Algos := registry.GetProtocolAlgorithms("protocol2")
	assert.Len(t, proto2Algos, 1)
	assert.Equal(t, "protocol2", proto2Algos[0].Protocol)

	// Get algorithms for non-existent protocol
	noneAlgos := registry.GetProtocolAlgorithms("nonexistent")
	assert.Empty(t, noneAlgos)
}

func TestHashRegistry_GetPrimaryAlgorithm(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	registry := core.GetHashRegistry()

	// Test empty registry
	primary := registry.GetPrimaryAlgorithm()
	assert.Equal(t, uint64(0), primary.Type)
	assert.Empty(t, primary.Name)

	// Register algorithms with different priorities
	algorithms := []core.HashAlgorithm{
		{
			Type:     0x12, // SHA2-256
			Name:     "SHA-256",
			Priority: 100,
			Protocol: "test",
		},
		{
			Type:     0xb220, // BLAKE2b-256
			Name:     "BLAKE2b-256",
			Priority: 90,
			Protocol: "test",
		},
		{
			Type:     0x1b, // SHA3-256
			Name:     "SHA3-256",
			Priority: 110,
			Protocol: "test2",
		},
	}

	for _, algo := range algorithms {
		err := registry.RegisterHashAlgorithm(algo)
		assert.NoError(t, err)
	}

	// Get primary algorithm (should be highest priority)
	primary = registry.GetPrimaryAlgorithm()
	assert.Equal(t, "SHA3-256", primary.Name)
	assert.Equal(t, 110, primary.Priority)
}

func TestResetHashAlgorithms(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	registry := core.GetHashRegistry()

	// Register some test hash algorithms
	err := registry.RegisterHashAlgorithm(core.HashAlgorithm{
		Type:     0x12, // SHA2-256
		Name:     "SHA-256",
		Priority: 100,
		Protocol: "test",
	})
	assert.NoError(t, err)

	err = registry.RegisterHashAlgorithm(core.HashAlgorithm{
		Type:     0xb220, // BLAKE2b-256
		Name:     "BLAKE2b-256",
		Priority: 90,
		Protocol: "test",
	})
	assert.NoError(t, err)

	// Check algorithms exist
	assert.Len(t, registry.GetHashAlgorithms(), 2)

	// Reset hash algorithms
	core.ResetHashAlgorithms()

	// Get the registry *after* the reset
	registryAfterReset := core.GetHashRegistry()

	// Check algorithms no longer exist in the new registry
	assert.Empty(t, registryAfterReset.GetHashAlgorithms())
	assert.Equal(t, uint64(0), registryAfterReset.GetPrimaryAlgorithm().Type)
}
