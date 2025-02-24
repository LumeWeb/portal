package core

import (
	"hash"
	"sort"
	"sync"
)

var (
	globalHashRegistry = NewHashRegistry()
)

// GetHashRegistry returns the global hash registry instance
func GetHashRegistry() HashRegistry {
	return globalHashRegistry
}

type HashAlgorithm struct {
	Type     uint64 // Hash algorithm type code (e.g. mh.SHA2_256 or mh.BLAKE2B_256)
	Name     string // Human-readable name (e.g. "SHA-256", "BLAKE2b-256")
	Priority int    // Priority for ordering
	Protocol string // Protocol that uses this hash (informational only)
	// Function to create a verifier for this algorithm
	NewVerifier func(hash StorageHash, proof []byte) (hash.Hash, error)
}

type HashRegistry interface {
	// Register a hash algorithm to be computed for all content
	RegisterHashAlgorithm(algo HashAlgorithm) error
	// Get all registered algorithms
	GetHashAlgorithms() []HashAlgorithm
	// Get algorithms for a specific protocol
	GetProtocolAlgorithms(protocol string) []HashAlgorithm
	// Get the primary/default algorithm (highest priority)
	GetPrimaryAlgorithm() HashAlgorithm
}

type HashRegistryDefault struct {
	algorithms []HashAlgorithm
	mu         sync.RWMutex
}

func NewHashRegistry() *HashRegistryDefault {
	return &HashRegistryDefault{
		algorithms: make([]HashAlgorithm, 0),
	}
}

func (r *HashRegistryDefault) RegisterHashAlgorithm(algo HashAlgorithm) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only check Type for uniqueness
	for _, existing := range r.algorithms {
		if existing.Type == algo.Type {
			return nil // Already registered, protocol is just metadata
		}
	}

	r.algorithms = append(r.algorithms, algo)

	// Sort by priority
	sort.Slice(r.algorithms, func(i, j int) bool {
		return r.algorithms[i].Priority > r.algorithms[j].Priority
	})

	return nil
}

func (r *HashRegistryDefault) GetHashAlgorithms() []HashAlgorithm {
	r.mu.RLock()
	defer r.mu.RUnlock()

	algorithms := make([]HashAlgorithm, len(r.algorithms))
	copy(algorithms, r.algorithms)
	return algorithms
}

func (r *HashRegistryDefault) GetProtocolAlgorithms(protocol string) []HashAlgorithm {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var protocolAlgos []HashAlgorithm
	for _, algo := range r.algorithms {
		if algo.Protocol == protocol {
			protocolAlgos = append(protocolAlgos, algo)
		}
	}
	return protocolAlgos
}

func (r *HashRegistryDefault) GetPrimaryAlgorithm() HashAlgorithm {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.algorithms) == 0 {
		return HashAlgorithm{}
	}
	return r.algorithms[0]
}
