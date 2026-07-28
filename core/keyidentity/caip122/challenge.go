package caip122

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ChallengeStore stores CAIP-122 nonces between the challenge and verify steps.
type ChallengeStore interface {
	Set(ctx context.Context, nonce string, domain string, ttl time.Duration) error
	Get(ctx context.Context, nonce string) (domain string, found bool, err error)
	Take(ctx context.Context, nonce string) (domain string, found bool, err error)
	Delete(ctx context.Context, nonce string) error
}

// MemoryChallengeStore is an in-process ChallengeStore for testing and
// single-instance deployments. A background goroutine periodically reaps
// expired entries to prevent unbounded memory growth.
type MemoryChallengeStore struct {
	mu         sync.RWMutex
	store      map[string]memoryChallenge
	maxEntries int
	stopChan   chan struct{}
	closeOnce  sync.Once
}

type memoryChallenge struct {
	domain    string
	expiresAt time.Time
}

// DefaultMaxChallengeEntries is the default cap on pending challenges.
const DefaultMaxChallengeEntries = 10000

func NewMemoryChallengeStore() *MemoryChallengeStore {
	m := &MemoryChallengeStore{
		store:      make(map[string]memoryChallenge),
		maxEntries: DefaultMaxChallengeEntries,
		stopChan:   make(chan struct{}),
	}
	go m.reapLoop()
	return m
}

// SetMaxEntriesForTest overrides the maxEntries cap. Intended for testing
// only — callers should use DefaultMaxChallengeEntries in production.
func (m *MemoryChallengeStore) SetMaxEntriesForTest(n int) {
	m.mu.Lock()
	m.maxEntries = n
	m.mu.Unlock()
}

// Close stops the background reaper goroutine. Safe to call multiple times.
func (m *MemoryChallengeStore) Close() error {
	m.closeOnce.Do(func() {
		close(m.stopChan)
	})
	return nil
}

func (m *MemoryChallengeStore) reapLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.reapExpired()
		}
	}
}

func (m *MemoryChallengeStore) reapExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reapExpiredLocked()
}

// reapExpiredLocked evicts expired entries. Caller must hold m.mu.
func (m *MemoryChallengeStore) reapExpiredLocked() {
	now := time.Now()
	for nonce, ch := range m.store {
		if now.After(ch.expiresAt) {
			delete(m.store, nonce)
		}
	}
}

func (m *MemoryChallengeStore) Set(ctx context.Context, nonce string, domain string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.maxEntries > 0 && len(m.store) >= m.maxEntries {
		// Bounded eviction: sample up to 64 random entries and evict
		// expired ones. This avoids an O(N) full scan under the write
		// lock while still reclaiming space from expired challenges.
		// If no expired entries are found, reject the write rather than
		// evicting a live nonce that an in-flight verification may need.
		if !m.tryEvictExpiredBounded(64) {
			return fmt.Errorf("caip122: challenge store capacity exceeded")
		}
	}
	m.store[nonce] = memoryChallenge{
		domain:    domain,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// tryEvictExpiredBounded samples up to maxSample random entries from the
// store and evicts any that have expired. Returns true if at least one
// entry was evicted. Caller must hold m.mu.
func (m *MemoryChallengeStore) tryEvictExpiredBounded(maxSample int) bool {
	now := time.Now()
	evicted := 0
	for nonce, ch := range m.store {
		if now.After(ch.expiresAt) {
			delete(m.store, nonce)
			evicted++
			if evicted >= maxSample {
				break
			}
		}
	}
	return evicted > 0
}


func (m *MemoryChallengeStore) Get(ctx context.Context, nonce string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.store[nonce]
	if !ok {
		return "", false, nil
	}
	if time.Now().After(ch.expiresAt) {
		delete(m.store, nonce)
		return "", false, nil
	}
	return ch.domain, true, nil
}

// Take atomically retrieves and deletes a nonce. This prevents the race
// condition where two concurrent requests both observe the same nonce via
// Get before either calls Delete.
func (m *MemoryChallengeStore) Take(ctx context.Context, nonce string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.store[nonce]
	if !ok {
		return "", false, nil
	}
	delete(m.store, nonce)
	if time.Now().After(ch.expiresAt) {
		return "", false, nil
	}
	return ch.domain, true, nil
}

func (m *MemoryChallengeStore) Delete(ctx context.Context, nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, nonce)
	return nil
}

// ChallengeConfig configures the CAIP-122 challenge service.
type ChallengeConfig struct {
	Domain     string
	TTL        time.Duration
	NonceBytes int
}

func DefaultChallengeConfig(domain string) ChallengeConfig {
	return ChallengeConfig{
		Domain:     domain,
		TTL:        5 * time.Minute,
		NonceBytes: 16,
	}
}

// ChallengeService issues and verifies CAIP-122 challenges.
type ChallengeService struct {
	store  ChallengeStore
	config ChallengeConfig
}

func NewChallengeService(store ChallengeStore, config ChallengeConfig) *ChallengeService {
	return &ChallengeService{store: store, config: config}
}

// GenerateChallenge issues a new CAIP-122 nonce for the configured domain.
func (s *ChallengeService) GenerateChallenge(ctx context.Context) (string, error) {
	nonceBytes := make([]byte, s.config.NonceBytes)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("caip122: failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	if err := s.store.Set(ctx, nonce, s.config.Domain, s.config.TTL); err != nil {
		return "", fmt.Errorf("caip122: failed to store nonce: %w", err)
	}

	return nonce, nil
}

// VerifyChallenge verifies a signed CAIP-122 message.
// Returns the verified Ethereum address (lowercase, 0x-prefixed) if valid.
func (s *ChallengeService) VerifyChallenge(ctx context.Context, message string, signature string) (string, error) {
	addr, _, err := s.VerifyChallengeWithChain(ctx, message, signature)
	return addr, err
}

// VerifyChallengeWithChain verifies a signed CAIP-122 message and returns both
// the recovered address and the chain_id from the parsed message. Callers should
// compare the returned chain_id against the expected chain_id to prevent
// cross-chain replay attacks.
func (s *ChallengeService) VerifyChallengeWithChain(ctx context.Context, message string, signature string) (string, string, error) {
	msg, err := ParseMessage(message)
	if err != nil {
		return "", "", fmt.Errorf("caip122: invalid message: %w", err)
	}

	// Take atomically retrieves and deletes the nonce, preventing concurrent
	// requests from replaying the same nonce.
	storedDomain, found, err := s.store.Take(ctx, msg.GetNonce())
	if err != nil {
		return "", "", fmt.Errorf("caip122: nonce lookup failed: %w", err)
	}
	if !found {
		return "", "", errors.New("caip122: invalid or expired nonce")
	}

	// Use the message's Verify method which handles domain matching, nonce
	// matching, time validation, and signature recovery in one call.
	domain := &storedDomain
	nonce := msg.GetNonce()
	address, err := msg.Verify(signature, domain, &nonce, nil)
	if err != nil {
		return "", "", err
	}

	return address, msg.GetChainIDString(), nil
}
