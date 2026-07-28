package keyidentity

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mr-tron/base58"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/keyidentity/caip122"
)

// SolanaMainnetGenesis is the CAIP-2 chain_id for Solana mainnet.
const SolanaMainnetChainID = "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

// SolanaHandler handles Solana address-based key identities.
//
// Key = Solana address (base58-encoded 32-byte Ed25519 public key).
// Metadata = JSON object, may contain "chain_id" (CAIP-2 string, e.g.,
// "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp").
//
// The handler implements the CAIP-122 Solana profile challenge/verify lifecycle:
//   - IssueChallenge: generates a nonce, stores it, and returns a SIWS message
//     template for the client to sign.
//   - VerifyProof: parses the signed SIWS message, validates nonce/domain/expiry,
//     verifies the Ed25519 signature against the claimed public key.
type SolanaHandler struct {
	mu             sync.RWMutex
	store          caip122.ChallengeStore
	domainResolver DomainResolver
	closeOnce      sync.Once
	closed         bool
}

// NewSolanaHandler creates a SolanaHandler with an in-memory challenge store
// and the given domain resolver. Pass CoreDomainResolver() to use the portal
// core domain, or a custom DomainResolver for plugin-specific subdomain logic.
func NewSolanaHandler(domainResolver DomainResolver) *SolanaHandler {
	if domainResolver == nil {
		domainResolver = CoreDomainResolver()
	}
	return &SolanaHandler{
		store:          caip122.NewMemoryChallengeStore(),
		domainResolver: domainResolver,
	}
}

// SetStore replaces the challenge store.
// The previous store is closed after the swap to prevent goroutine leaks.
//
// SetStore is safe to call concurrently with IssueChallenge/VerifyProof: the
// old store is closed only after the write lock is released, so any in-flight
// readers holding the RLock will complete before the close.
func (h *SolanaHandler) SetStore(store caip122.ChallengeStore) {
	if store == nil {
		return
	}
	h.mu.Lock()
	oldStore := h.store
	h.store = store
	h.mu.Unlock()
	// Close the old store outside the lock so in-flight readers (holding
	// RLock on the old store reference) complete before it's closed.
	if closer, ok := oldStore.(io.Closer); ok {
		_ = closer.Close()
	}
}

// Close stops the background reaper goroutine.
// Safe to call multiple times. After Close, IssueChallenge and VerifyProof
// return an error.
func (h *SolanaHandler) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		if closer, ok := h.store.(io.Closer); ok {
			_ = closer.Close()
		}
		h.closed = true
		h.mu.Unlock()
	})
	return nil
}

// NormalizeKey validates and canonicalizes a Solana address.
// The key must be a base58-encoded 32-byte Ed25519 public key.
// Returns the canonical base58 re-encoding to ensure consistent representation.
func (h *SolanaHandler) NormalizeKey(key string) (string, error) {
	decoded, err := base58.Decode(key)
	if err != nil {
		return "", fmt.Errorf("solana: invalid base58 address: %w", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return "", fmt.Errorf("solana: invalid address length: expected %d bytes, got %d", ed25519.PublicKeySize, len(decoded))
	}
	// Re-encode to ensure canonical form
	return base58.Encode(decoded), nil
}

// solanaMetadata is the typed schema for Solana KeyIdentity metadata.
type solanaMetadata struct {
	ChainID string `json:"chain_id,omitempty"`
}

// ValidateMetadata validates the metadata JSON for a Solana key identity.
// If chain_id is present, it must be a valid CAIP-2 solana: identifier.
// If absent, defaults to mainnet.
func (h *SolanaHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	if len(metadata) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var m solanaMetadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil, fmt.Errorf("solana: invalid metadata JSON: %w", err)
	}

	if m.ChainID != "" {
		if !strings.HasPrefix(m.ChainID, "solana:") {
			return nil, fmt.Errorf("solana: chain_id must be CAIP-2 format (solana:<genesis>), got %q", m.ChainID)
		}
		genesis := strings.TrimPrefix(m.ChainID, "solana:")
		if genesis == "" {
			return nil, fmt.Errorf("solana: chain_id must be CAIP-2 format (solana:<genesis>), got %q", m.ChainID)
		}
	}

	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("solana: failed to canonicalize metadata: %w", err)
	}
	return json.RawMessage(canonical), nil
}

// extractChainID validates metadata and extracts the chain_id.
// Returns the validated chain_id, or SolanaMainnetChainID if not present.
func (h *SolanaHandler) extractChainID(metadata json.RawMessage) (string, error) {
	validated, err := h.ValidateMetadata(metadata)
	if err != nil {
		return "", err
	}

	chainID := SolanaMainnetChainID
	var m solanaMetadata
	if err := json.Unmarshal(validated, &m); err == nil && m.ChainID != "" {
		chainID = m.ChainID
	}
	return chainID, nil
}

// IssueChallenge generates a CAIP-122 challenge for proving ownership of the
// given Solana address. Returns a JSON payload with nonce and SIWS message.
func (h *SolanaHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
	normalized, err := h.NormalizeKey(key)
	if err != nil {
		return nil, err
	}

	chainID, err := h.extractChainID(metadata)
	if err != nil {
		return nil, err
	}

	domain := h.domainResolver.ResolveDomain(ctx)
	if domain == "" {
		return nil, fmt.Errorf("solana: cannot determine domain for CAIP-122 challenge")
	}

	// Hold the lock across the entire challenge operation so a concurrent
	// SetStore cannot close or replace the store mid-operation. Check closed
	// state inside the lock to avoid a TOCTOU race with Close.
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return nil, fmt.Errorf("solana: handler is closed")
	}

	challenge := caip122.NewChallengeService(h.store, caip122.DefaultChallengeConfig(domain))

	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		return nil, fmt.Errorf("solana: failed to generate challenge: %w", err)
	}

	message, err := caip122.FormatSolanaMessage(normalized, domain, nonce, chainID, caip122.DefaultChallengeConfig(domain).TTL)
	if err != nil {
		return nil, fmt.Errorf("solana: failed to construct SIWS message: %w", err)
	}

	response, err := caip122.EncodeSolanaChallengeResponse(nonce, message)
	if err != nil {
		return nil, fmt.Errorf("solana: failed to marshal challenge response: %w", err)
	}

	return response, nil
}

// VerifyProof verifies a CAIP-122 signed message as proof of Solana address ownership.
//
// The proof parameter is a JSON payload:
//   {"message": "<SIWS plaintext>", "signature": "<base58 Ed25519 sig>"}
//
// The handler validates nonce/domain/expiry, verifies the Ed25519 signature
// against the claimed public key, and compares the address in the message
// to the claimed key.
func (h *SolanaHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	var payload struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(proof, &payload); err != nil {
		return fmt.Errorf("solana: invalid proof payload: %w", err)
	}

	if payload.Message == "" || payload.Signature == "" {
		return fmt.Errorf("solana: proof payload must contain 'message' and 'signature' fields")
	}

	normalized, err := h.NormalizeKey(key)
	if err != nil {
		return err
	}

	chainID, err := h.extractChainID(metadata)
	if err != nil {
		return err
	}

	// Verify the chain_id in the signed message matches the registered metadata
	// before consuming the nonce, so an invalid chain_id doesn't delete a
	// legitimate challenge.
	parsed, err := caip122.ParseSolanaMessage(payload.Message)
	if err != nil {
		return fmt.Errorf("solana: invalid proof message: %w", err)
	}
	msgChainID := "solana:" + parsed.GetChainID()
	if msgChainID != chainID {
		return fmt.Errorf("solana: chain_id mismatch (expected %s, got %s)", chainID, msgChainID)
	}

	// Hold the lock across the entire verify operation so a concurrent
	// SetStore cannot close or replace the store mid-operation. Check closed
	// state inside the lock to avoid a TOCTOU race with Close.
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return fmt.Errorf("solana: handler is closed")
	}

	address, err := caip122.VerifySolanaChallengeParsed(ctx, h.store, parsed, payload.Signature)
	if err != nil {
		return fmt.Errorf("solana: proof verification failed: %w", err)
	}

	// Verify the address from the signed message matches the claimed key.
	// Both should be canonical base58 at this point.
	if address != normalized {
		return fmt.Errorf("solana: address mismatch (expected %s, got %s)", normalized, address)
	}

	return nil
}
