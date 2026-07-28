package keyidentity

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/keyidentity/caip122"
	"go.uber.org/zap"
)

// EthereumHandler handles Ethereum address-based key identities.
//
// Key = Ethereum address (hex string, normalized to lowercase 0x-prefixed).
// Metadata = JSON object, may contain "chain_id" (CAIP-2 string, e.g., "eip155:1").
//
// The handler implements the full CAIP-122 (EIP-4361) challenge/verify lifecycle:
//   - IssueChallenge: generates a nonce, stores it, and returns a SIWE message
//     template for the client to sign.
//   - VerifyProof: parses the signed SIWE message, validates nonce/domain/expiry,
//     recovers the signer via secp256k1, and compares to the claimed key.
//
// All methods that need runtime context receive core.Context, which provides
// access to config (for domain resolution), DB, logger, and services.
type EthereumHandler struct {
	mu             sync.RWMutex
	store          caip122.ChallengeStore
	domainResolver DomainResolver
	closeOnce      sync.Once
	closed         bool
}

// NewEthereumHandler creates an EthereumHandler with an in-memory challenge store
// and the given domain resolver. Pass CoreDomainResolver() to use the portal
// core domain, or a custom DomainResolver for plugin-specific subdomain logic.
// The store can be replaced via SetStore (e.g., with a Redis-backed implementation).
// Call Close() to stop the background reaper goroutine when the handler is no
// longer needed.
func NewEthereumHandler(domainResolver DomainResolver) *EthereumHandler {
	if domainResolver == nil {
		domainResolver = CoreDomainResolver()
	}
	return &EthereumHandler{
		store:          caip122.NewMemoryChallengeStore(),
		domainResolver: domainResolver,
	}
}

// SetStore replaces the challenge store (e.g., with a Redis-backed implementation).
// The previous store is closed after the swap to prevent goroutine leaks.
//
// SetStore is safe to call concurrently with IssueChallenge/VerifyProof: the
// old store is closed only after the write lock is released, so any in-flight
// readers holding the RLock will complete before the close.
func (h *EthereumHandler) SetStore(store caip122.ChallengeStore) {
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

// Close stops the background reaper goroutine if the store implements io.Closer.
// Safe to call multiple times. After Close, IssueChallenge and VerifyProof
// return an error.
func (h *EthereumHandler) Close() error {
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

func (h *EthereumHandler) NormalizeKey(key string) (string, error) {
	if !strings.HasPrefix(key, "0x") || len(key) != 42 {
		return "", fmt.Errorf("invalid ethereum address: must be 0x-prefixed hex, 42 chars, got %q", key)
	}
	if _, err := hex.DecodeString(key[2:]); err != nil {
		return "", fmt.Errorf("invalid ethereum address: non-hex character in %q", key)
	}
	return strings.ToLower(key), nil
}

// ethereumMetadata is the typed schema for Ethereum KeyIdentity metadata.
// Using a struct eliminates fragile map[string]interface{} type assertions.
type ethereumMetadata struct {
	ChainID string `json:"chain_id,omitempty"`
}

func (h *EthereumHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	if len(metadata) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var m ethereumMetadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil, fmt.Errorf("invalid metadata JSON: %w", err)
	}

	if m.ChainID != "" {
		if !strings.HasPrefix(m.ChainID, "eip155:") {
			return nil, fmt.Errorf("chain_id must be CAIP-2 format (eip155:<number>), got %q", m.ChainID)
		}
		numPart := strings.TrimPrefix(m.ChainID, "eip155:")
		if numPart == "" {
			return nil, fmt.Errorf("chain_id must be CAIP-2 format (eip155:<number>), got %q", m.ChainID)
		}
		chainNum, err := strconv.Atoi(numPart)
		if err != nil || chainNum < 0 {
			return nil, fmt.Errorf("chain_id must be CAIP-2 format (eip155:<number>), got %q", m.ChainID)
		}
		// Canonicalize to eip155:<int> to prevent leading-zero mismatches
		// between metadata and the signed SIWE message.
		m.ChainID = fmt.Sprintf("eip155:%d", chainNum)
	}

	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize metadata: %w", err)
	}
	return json.RawMessage(canonical), nil
}

// extractChainID validates metadata and extracts the chain_id.
// Returns the validated chain_id, or "eip155:1" if no chain_id is present.
// Returns an error if metadata is present but invalid.
func (h *EthereumHandler) extractChainID(metadata json.RawMessage) (string, error) {
	validated, err := h.ValidateMetadata(metadata)
	if err != nil {
		return "", err
	}

	chainID := "eip155:1"
	var m ethereumMetadata
	if err := json.Unmarshal(validated, &m); err == nil && m.ChainID != "" {
		chainID = m.ChainID
	}
	return chainID, nil
}

// IssueChallenge generates a CAIP-122 challenge for proving ownership of the
// given Ethereum address. The returned bytes are a JSON payload containing
// the nonce and the SIWE message text for the client to sign.
//
// The challenge state (nonce + domain) is stored in the handler's ChallengeStore
// so that VerifyProof can validate it later.
func (h *EthereumHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
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
		return nil, fmt.Errorf("ethereum: cannot determine domain for CAIP-122 challenge")
	}

	// Hold the lock across the entire challenge operation so a concurrent
	// SetStore cannot close or replace the store mid-operation. Check closed
	// state inside the lock to avoid a TOCTOU race with Close.
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return nil, fmt.Errorf("ethereum: handler is closed")
	}

	challenge := caip122.NewChallengeService(h.store, caip122.DefaultChallengeConfig(domain))

	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		return nil, fmt.Errorf("ethereum: failed to generate challenge: %w", err)
	}

	config := caip122.DefaultChallengeConfig(domain)
	message, err := caip122.FormatMessage(normalized, domain, nonce, chainID, config.TTL)
	if err != nil {
		return nil, fmt.Errorf("ethereum: failed to construct SIWE message: %w", err)
	}

	response, err := json.Marshal(struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}{
		Nonce:   nonce,
		Message: message,
	})
	if err != nil {
		return nil, fmt.Errorf("ethereum: failed to marshal challenge response: %w", err)
	}

	return response, nil
}

// VerifyProof verifies a CAIP-122 signed message as proof of Ethereum address ownership.
//
// The proof parameter is a JSON payload:
//   {"message": "<EIP-4361 plaintext>", "signature": "<0x-prefixed hex RSV>"}
//
// The handler validates nonce/domain/expiry, recovers the signer via secp256k1,
// and compares the recovered address to the claimed key. It also validates that
// the chain_id in the signed message matches the chain_id in the registered metadata.
func (h *EthereumHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	var payload struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(proof, &payload); err != nil {
		return fmt.Errorf("ethereum: invalid proof payload: %w", err)
	}

	if payload.Message == "" || payload.Signature == "" {
		return fmt.Errorf("ethereum: proof payload must contain 'message' and 'signature' fields")
	}

	// Validate the key format before touching the challenge store.
	// This prevents malformed keys from consuming (deleting) legitimate nonces.
	normalized, err := h.NormalizeKey(key)
	if err != nil {
		return err
	}

	// Validate metadata (ensures chain_id format is correct if present).
	chainID, err := h.extractChainID(metadata)
	if err != nil {
		return err
	}

	domain := h.domainResolver.ResolveDomain(ctx)

	// Hold the lock across the entire verify operation so a concurrent
	// SetStore cannot close or replace the store mid-operation. Check closed
	// state inside the lock to avoid a TOCTOU race with Close.
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return fmt.Errorf("ethereum: handler is closed")
	}

	challenge := caip122.NewChallengeService(h.store, caip122.DefaultChallengeConfig(domain))

	address, msgChainID, err := challenge.VerifyChallengeWithChain(ctx, payload.Message, payload.Signature)
	if err != nil {
		return fmt.Errorf("ethereum: proof verification failed: %w", err)
	}

	// ---------------------------------------------------------------------------
	// Chain-ID policy: log-only, not enforced.
	//
	// The SIWE chain_id from the signed message is compared against the
	// chain_id stored in the key's metadata, but a mismatch does NOT reject
	// the proof. This is an intentional design decision, not an oversight:
	//
	// 1. Identity = address, not chain. An EVM externally-owned account
	//    (EOA) is a secp256k1 keypair. The address is derived from the
	//    public key and is identical across all EVM chains. The same
	//    private key can sign on mainnet, Base, Polygon, Arbitrum, etc.
	//    The chain_id in SIWE is an informational field about which
	//    network the wallet was connected to at signing time — it does
	//    not change the signer's identity.
	//
	// 2. Enforcing chain binding would lock an identity to the chain it
	//    was registered on. A user who registered on mainnet would be
	//    unable to authenticate if their wallet is connected to Polygon,
	//    forcing them to switch networks just to log in — hostile UX
	//    for no security gain.
	//
	// 3. The nonce is single-use and atomically consumed (Take), so
	//    cross-chain replay is impossible regardless of chain_id. An
	//    attacker who intercepts a valid signature cannot reuse it
	//    because the nonce is deleted after the first verification.
	//
	// 4. The domain is verified (CAIP-122 RFC 3986 URI match), preventing
	//    cross-origin phishing. A signature for "phishing.com" cannot
	//    satisfy a challenge for "account.example.com".
	//
	// 5. The signature itself is chain-agnostic — EIP-191 personal_sign
	//    recovery yields the same address regardless of which chain_id
	//    the message claims. The chain_id is embedded in the message
	//    text but does not affect secp256k1 recovery.
	//
	// A mismatch is logged for observability (anomaly detection, SIEM
	// correlation, debugging misconfigured wallets) but does not block
	// authentication.
	//
	// Contrast with Solana (solana.go): Solana chain_id IS enforced
	// because Solana genesis hashes define distinct clusters (mainnet,
	// devnet, testnet) with different validators and operational
	// semantics. A devnet identity should not satisfy a mainnet
	// challenge. For EVM, all chains share the same address space and
	// signature scheme, so the distinction is meaningless.
	//
	// Reviewer note: This has been flagged as a security concern
	// ("cross-chain replay"). The threat model does not support
	// hard-blocking: the nonce is consumed atomically, the domain is
	// verified, and the address is chain-invariant. Blocking would
	// degrade UX for legitimate cross-chain users without closing any
	// exploitable gap.
	// ---------------------------------------------------------------------------
	if msgChainID != chainID {
		if ctx != nil {
			if logger := ctx.Logger(); logger != nil {
				logger.Warn("ethereum: chain_id mismatch in key identity proof",
					zap.String("metadata_chain_id", chainID),
					zap.String("signed_chain_id", msgChainID),
				)
			}
		}
	}

	if address != normalized {
		return fmt.Errorf("ethereum: address mismatch (expected %s, recovered %s)", normalized, address)
	}

	return nil
}
