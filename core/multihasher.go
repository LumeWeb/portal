package core

import (
	"github.com/gammazero/workerpool"
	mh "github.com/multiformats/go-multihash"
	mhcore "github.com/multiformats/go-multihash/core" // Import the core package
	"hash"
	"io"
	"sync"
)

// HashFactory is an interface for creating hash.Hash instances.
// This allows us to inject different implementations for testing.
// This interface is now primarily for internal use and testing.
type HashFactory interface {
	// GetHasher returns a hash.Hash for the given code, using the default size.
	GetHasher(code uint64) (hash.Hash, error)
	// GetVariableHasher returns a hash.Hash for the given code and size hint.
	GetVariableHasher(code uint64, sizeHint int) (hash.Hash, error)
}

// DefaultHashFactory is a HashFactory that wraps the standard go-multihash functions.
type DefaultHashFactory struct{}

func (f DefaultHashFactory) GetHasher(code uint64) (hash.Hash, error) {
	return mhcore.GetHasher(code)
}

func (f DefaultHashFactory) GetVariableHasher(code uint64, sizeHint int) (hash.Hash, error) {
	return mhcore.GetVariableHasher(code, sizeHint)
}

type MultiHasher struct {
	writers     []io.Writer
	hashes      []hash.Hash
	algos       []HashAlgorithm
	pool        *workerpool.WorkerPool
	mutex       sync.Mutex // Protects access to hashes during concurrent writes
	verifying   bool       // If true, we're in verify mode
	verifyReqs  []*VerifyRequest // Store verify requests for Sums()
	hashFactory HashFactory // Injected dependency for creating hashers
	source      io.Reader  // Source reader for passthrough mode
}

type HashResult struct {
	Hash      StorageHash
	Algorithm HashAlgorithm
	Proof     []byte // Set when generated during hashing or from VerifyRequest
	Verified  bool   // Set in verify mode
	Error     error  // Any errors during hashing, proof generation, or verification
}

// VerifyRequest contains the data needed to verify a hash
type VerifyRequest struct {
	Algorithm HashAlgorithm
	Hash      StorageHash
	Proof     []byte
}

// NewMultiHasher creates a new MultiHasher
// If verifyReqs is non-empty, the hasher will be in verify mode
// An optional HashFactory can be provided for testing purposes.
func NewMultiHasher(verifyReqs ...*VerifyRequest) *MultiHasher {
	return NewMultiHasherWithFactory(DefaultHashFactory{}, verifyReqs...)
}

// NewMultiHasherFromReader creates a new MultiHasher that will hash data as it's read from the provided reader.
// The returned MultiHasher implements io.Reader and can be used as a passthrough.
func NewMultiHasherFromReader(r io.Reader) (*MultiHasher, error) {
	hasher := NewMultiHasher()
	
	// If the input is seekable, ensure we start from the beginning
	if seeker, ok := r.(io.ReadSeeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			hasher.Close()
			return nil, err
		}
	}

	// Store the source reader
	hasher.source = r
	
	return hasher, nil
}

// NewMultiHasherWithFactory creates a new MultiHasher with a specific HashFactory.
// This is primarily for testing.
func NewMultiHasherWithFactory(factory HashFactory, verifyReqs ...*VerifyRequest) *MultiHasher {
	registry := GetHashRegistry()
	algos := registry.GetHashAlgorithms()

	verifying := len(verifyReqs) > 0
	pool := workerpool.New(len(algos))

	hasher := &MultiHasher{
		writers:     make([]io.Writer, len(algos)),
		hashes:      make([]hash.Hash, len(algos)),
		algos:       algos,
		pool:        pool,
		verifying:   verifying,
		verifyReqs:  verifyReqs, // Store verify requests
		hashFactory: factory,
	}

	for i, algo := range algos {
		if verifying {
			// In verify mode, look for a verify request for this algorithm
			var matchingReq *VerifyRequest
			for _, req := range verifyReqs {
				if req.Algorithm.Type == algo.Type {
					matchingReq = req
					break
				}
			}

			if matchingReq != nil {
				// Try specialized verifier if available
				if algo.NewVerifier != nil {
					h, err := algo.NewVerifier(matchingReq.Hash, matchingReq.Proof)
					if err == nil {
						hasher.hashes[i] = h
						hasher.writers[i] = h
						continue // Move to the next algorithm
					}
					// If specialized verifier fails, fall through to standard hasher
				}

				// Fall back to standard hasher with proof provider capability
				// Use GetVariableHasher with the expected digest length from the StorageHash
				decodedHash, err := mh.Decode(matchingReq.Hash.Multihash())
				if err != nil {
					// If we can't decode the hash, we can't get the size hint, skip this algo
					continue
				}

				h, err := factory.GetVariableHasher(algo.Type, decodedHash.Length)
				if err != nil {
					// If we can't get the hasher, skip this algo
					continue
				}

				hasher.hashes[i] = h
				hasher.writers[i] = h

				// Don't set proof here - will be set in Sums() when needed
			}
			// If no verify request for this algo, this hasher remains nil
		} else {
			// Create mode - normal hashers for all algorithms
			h, err := factory.GetHasher(algo.Type)
			if err != nil {
				// If we can't get the hasher, this hasher remains nil
				continue
			}
			hasher.hashes[i] = h
			hasher.writers[i] = h
		}
	}

	return hasher
}

func (m *MultiHasher) Write(p []byte) (n int, err error) {
	var wg sync.WaitGroup
	var errMutex sync.Mutex
	var writeErr error

	for _, w := range m.writers {
		if w != nil {
			wg.Add(1)

			writer := w
			// Create a new copy of the data for each worker
			dataCopy := make([]byte, len(p))
			copy(dataCopy, p)

			// Submit task to worker pool
			m.pool.Submit(func() {
				defer wg.Done()

				_, err := writer.Write(dataCopy)
				if err != nil {
					errMutex.Lock()
					// Only store the first error encountered
					if writeErr == nil {
						writeErr = err
					}
					errMutex.Unlock()
				}
			})
		}
	}

	wg.Wait()

	if writeErr != nil {
		return 0, writeErr
	}

	return len(p), nil
}

func (m *MultiHasher) Sums() []HashResult {
	results := make([]HashResult, 0)

	for i, h := range m.hashes {
		if h == nil {
			continue
		}

		sum := h.Sum(nil)
		var proof []byte
		var verified bool
		var verifyErr error

		if m.verifying {
			// In verify mode, get proof from the original verify request
			var matchingReq *VerifyRequest
			for _, req := range m.verifyReqs {
				if req.Algorithm.Type == m.algos[i].Type {
					matchingReq = req
					break
				}
			}
			if matchingReq != nil {
				proof = matchingReq.Proof
				
				// Set proof right before verification if the hasher supports it
				if provider, ok := h.(HashProofProvider); ok {
					if err := provider.SetProof(proof); err != nil {
						verifyErr = err
						continue
					}
				}
			}

			// Check if the hasher can verify the proof
			if verifier, ok := h.(HashProofVerifier); ok {
				verified, verifyErr = verifier.VerifyProof()
			} else {
				// If not a verifier, it's not considered verified by this hasher
				verified = false
				verifyErr = nil // No verification error if it doesn't implement the interface
			}
		} else {
			// In create mode, get proof from the hasher if it generates one
			if generator, ok := h.(HashProofGenerator); ok {
				proof = generator.GetProof()
			}
			verified = false // Not in verify mode
			verifyErr = nil
		}

		result := HashResult{
			Hash:      NewStorageHash(sum, m.algos[i].Type, 0, proof),
			Algorithm: m.algos[i],
			Proof:     proof, // Proof from either generation or verify request
			Verified:  verified,
			Error:     verifyErr, // Only verification errors are reported here
		}

		results = append(results, result)
	}
	return results
}

func (m *MultiHasher) Read(p []byte) (n int, err error) {
	if m.source == nil {
		return 0, io.EOF
	}

	n, err = m.source.Read(p)
	if n > 0 {
		// Hash the data we just read
		if _, writeErr := m.Write(p[:n]); writeErr != nil {
			return n, writeErr
		}
	}
	return n, err
}

func (m *MultiHasher) Close() {
	// StopWait waits for all submitted tasks to complete.
	m.pool.StopWait()
}

// SetHashes sets the internal hash implementations.
// This method is primarily intended for testing purposes to inject mock hashers.
// It should not be used in production code.
func (m *MultiHasher) SetHashes(hashes []hash.Hash) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hashes = hashes
}

// SetWriters sets the internal writer implementations.
// This method is primarily intended for testing purposes to inject mock writers.
// It should not be used in production code.
func (m *MultiHasher) SetWriters(writers []io.Writer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.writers = writers
}

// HashProofGenerator represents a hash implementation that generates proofs during hashing
type HashProofGenerator interface {
	GetProof() []byte
}

// HashProofProvider represents a hash implementation that accepts proof data
type HashProofProvider interface {
	SetProof(proof []byte) error
}

// HashProofVerifier represents a hash implementation that can verify proofs
type HashProofVerifier interface {
	VerifyProof() (bool, error)
}

type TestingHashWithProof interface {
	hash.Hash
	HashProofGenerator
	HashProofProvider
}

type TestingHashProofGenerator interface {
	hash.Hash
	HashProofGenerator
}

type TestingHashProofProvider interface {
	hash.Hash
	HashProofProvider
}

type TestingHashProofVerifier interface {
	hash.Hash
	HashProofVerifier
}

type TestingTestingHashWithProof interface {
	hash.Hash
	HashProofGenerator
	HashProofProvider
	HashProofVerifier
}

type TestingHash interface {
	hash.Hash
}
