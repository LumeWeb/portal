package core

import (
	"github.com/gammazero/workerpool"
	mh "github.com/multiformats/go-multihash"
	"hash"
	"io"
	"sync"
)

type MultiHasher struct {
	writers   []io.Writer
	hashes    []hash.Hash
	algos     []HashAlgorithm
	pool      *workerpool.WorkerPool
	mutex     sync.Mutex // Protects access to hashes during concurrent writes
	verifying bool       // If true, we're in verify mode
}

type HashResult struct {
	Hash      StorageHash
	Algorithm HashAlgorithm
	Proof     []byte // Set when generated during hashing
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
func NewMultiHasher(verifyReqs ...*VerifyRequest) *MultiHasher {
	registry := GetHashRegistry()
	algos := registry.GetHashAlgorithms()

	verifying := len(verifyReqs) > 0
	pool := workerpool.New(len(algos))

	hasher := &MultiHasher{
		writers:   make([]io.Writer, len(algos)),
		hashes:    make([]hash.Hash, len(algos)),
		algos:     algos,
		pool:      pool,
		verifying: verifying,
	}

	for i, algo := range algos {
		if verifying {
			// In verify mode, look for a verify request for this algorithm
			for _, req := range verifyReqs {
				if req.Algorithm.Type == algo.Type {
					// Try specialized verifier if available
					if algo.NewVerifier != nil {
						h, err := algo.NewVerifier(req.Hash, req.Proof)
						if err == nil {
							hasher.hashes[i] = h
							hasher.writers[i] = h
							break
						}
					}

					// Fall back to standard hasher with proof provider capability
					h, err := mh.GetHasher(algo.Type)
					if err != nil {
						continue
					}

					hasher.hashes[i] = h
					hasher.writers[i] = h

					// Provide proof data if the hasher supports it
					if provider, ok := h.(HashProofProvider); ok {
						_ = provider.SetProof(req.Proof)
					}
					break
				}
			}

			// If no verify request for this algo, set up normal hasher
			if hasher.hashes[i] == nil {
				h, err := mh.GetHasher(algo.Type)
				if err != nil {
					continue
				}
				hasher.hashes[i] = h
				hasher.writers[i] = h
			}
		} else {
			// Create mode - normal hashers for all algorithms
			h, err := mh.GetHasher(algo.Type)
			if err != nil {
				continue
			}
			hasher.hashes[i] = h
			hasher.writers[i] = h
		}
	}

	return hasher
}

func (m *MultiHasher) Write(p []byte) (n int, err error) {
	// Make a copy of the data since p might be reused by the caller
	dataCopy := make([]byte, len(p))
	copy(dataCopy, p)

	// Use a WaitGroup to wait for all hash operations to complete
	var wg sync.WaitGroup
	var errMutex sync.Mutex
	var writeErr error

	for _, w := range m.writers {
		if w != nil {
			wg.Add(1)

			// Capture loop variables to avoid closure problems
			writer := w

			// Submit task to worker pool
			m.pool.Submit(func() {
				defer wg.Done()

				// Perform the write operation
				_, err := writer.Write(dataCopy)
				if err != nil {
					errMutex.Lock()
					writeErr = err
					errMutex.Unlock()
				}
			})
		}
	}

	// Wait for all hash operations to complete
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

		// Check for proof generation
		if generator, ok := h.(HashProofGenerator); ok {
			proof = generator.GetProof()
		}

		result := HashResult{
			Hash:      NewStorageHash(sum, m.algos[i].Type, 0, proof),
			Algorithm: m.algos[i],
			Proof:     proof,
		}

		// Check for verification if in verify mode
		if m.verifying {
			if verifier, ok := h.(HashProofVerifier); ok {
				verified, err := verifier.VerifyProof()
				result.Verified = verified
				result.Error = err
			}
		}

		results = append(results, result)
	}
	return results
}

func (m *MultiHasher) Close() {
	// Stop the worker pool when the MultiHasher is no longer needed
	m.pool.StopWait()
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
