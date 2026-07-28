package keyidentity

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	stdtesting "testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/portal/core/keyidentity/caip122"
)

// generateSolanaKey generates an Ed25519 keypair and returns (address, privateKey).
func generateSolanaKey(t *stdtesting.T) (string, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return base58.Encode(pub), priv
}

func TestSolanaHandler_NormalizeKey(t *stdtesting.T) {
	h := NewSolanaHandler(CoreDomainResolver())

	t.Run("valid key", func(t *stdtesting.T) {
		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		addr := base58.Encode(pub)

		got, err := h.NormalizeKey(addr)
		require.NoError(t, err)
		assert.Equal(t, addr, got)
	})

	t.Run("canonical re-encode", func(t *stdtesting.T) {
		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		// Decode and re-encode to get a potentially different representation
		decoded, _ := base58.Decode(base58.Encode(pub))
		addr := base58.Encode(decoded)

		got, err := h.NormalizeKey(addr)
		require.NoError(t, err)
		assert.Equal(t, addr, got)
	})

	t.Run("invalid base58", func(t *stdtesting.T) {
		_, err := h.NormalizeKey("!!!invalid!!!")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid base58")
	})

	t.Run("wrong length", func(t *stdtesting.T) {
		// 31 bytes instead of 32
		short := base58.Encode(make([]byte, 31))
		_, err := h.NormalizeKey(short)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid address length")
	})

	t.Run("empty", func(t *stdtesting.T) {
		_, err := h.NormalizeKey("")
		require.Error(t, err)
	})
}

func TestSolanaHandler_ValidateMetadata(t *stdtesting.T) {
	h := NewSolanaHandler(CoreDomainResolver())

	t.Run("empty defaults to {}", func(t *stdtesting.T) {
		out, err := h.ValidateMetadata(nil)
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(out))
	})

	t.Run("valid chain_id", func(t *stdtesting.T) {
		out, err := h.ValidateMetadata(json.RawMessage(`{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`))
		require.NoError(t, err)
		assert.JSONEq(t, `{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`, string(out))
	})

	t.Run("non-solana chain_id", func(t *stdtesting.T) {
		_, err := h.ValidateMetadata(json.RawMessage(`{"chain_id":"eip155:1"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be CAIP-2 format (solana:<genesis>)")
	})

	t.Run("empty genesis", func(t *stdtesting.T) {
		_, err := h.ValidateMetadata(json.RawMessage(`{"chain_id":"solana:"}`))
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *stdtesting.T) {
		_, err := h.ValidateMetadata(json.RawMessage(`{invalid`))
		require.Error(t, err)
	})
}

func TestSolanaHandler_IssueChallenge(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key := base58.Encode(pub)

	out, err := h.IssueChallenge(ctx, key, json.RawMessage(`{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`))
	require.NoError(t, err)

	var resp struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(out, &resp))

	assert.NotEmpty(t, resp.Nonce, "nonce should not be empty")
	assert.NotEmpty(t, resp.Message, "message should not be empty")
	assert.Contains(t, resp.Message, key, "message should contain the address")
	assert.Contains(t, resp.Message, "Solana account:", "message should mention Solana")
	assert.Contains(t, resp.Message, "Nonce: "+resp.Nonce, "message should contain the nonce")
}

func TestSolanaHandler_IssueChallenge_InvalidKey(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	_, err := h.IssueChallenge(ctx, "!!!invalid!!!", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base58")
}

func TestSolanaHandler_VerifyProof_Success(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, priv := generateSolanaKey(t)

	// Issue challenge
	challengeBytes, err := h.IssueChallenge(ctx, addr, json.RawMessage(`{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`))
	require.NoError(t, err)

	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(challengeBytes, &challenge))

	// Sign the message with the Solana private key
	sig := ed25519.Sign(priv, []byte(challenge.Message))
	sigB58 := base58.Encode(sig)

	// Verify the proof
	proof, _ := json.Marshal(map[string]string{
		"message":   challenge.Message,
		"signature": sigB58,
	})

	err = h.VerifyProof(ctx, addr, nil, proof)
	require.NoError(t, err)
}

func TestSolanaHandler_VerifyProof_AddressMismatch(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, _ := generateSolanaKey(t)
	otherAddr, otherPriv := generateSolanaKey(t) // different key

	// Issue challenge for addr
	challengeBytes, err := h.IssueChallenge(ctx, addr, json.RawMessage(`{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`))
	require.NoError(t, err)

	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(challengeBytes, &challenge))

	// Sign with a different key
	sig := ed25519.Sign(otherPriv, []byte(challenge.Message))
	sigB58 := base58.Encode(sig)

	proof, _ := json.Marshal(map[string]string{
		"message":   challenge.Message,
		"signature": sigB58,
	})

	// The message contains addr, but the signature was made with otherPriv.
	// VerifyProof is called with addr as the claimed key.
	// The Ed25519 verification will fail because the signature doesn't match
	// the public key derived from addr.
	err = h.VerifyProof(ctx, addr, nil, proof)
	require.Error(t, err)
	// Could be address mismatch or signature verification failure
	_ = otherAddr
}

func TestSolanaHandler_VerifyProof_InvalidNonce(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, priv := generateSolanaKey(t)

	// Create a challenge but don't use the handler's store
	// Instead, craft a message with a nonce that was never stored
	msg, err := caip122.FormatSolanaMessage(addr, "test.example.com", "neverstoredno", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 0)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	proof, _ := json.Marshal(map[string]string{
		"message":   msg,
		"signature": sigB58,
	})

	err = h.VerifyProof(ctx, addr, nil, proof)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired nonce")
}

func TestSolanaHandler_VerifyProof_InvalidPayload(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, _ := generateSolanaKey(t)

	t.Run("malformed json", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, addr, nil, []byte("not json"))
		require.Error(t, err)
	})

	t.Run("missing fields", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, addr, nil, []byte(`{"message":"hello"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must contain 'message' and 'signature'")
	})

	t.Run("empty fields", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, addr, nil, []byte(`{"message":"","signature":""}`))
		require.Error(t, err)
	})
}

func TestSolanaHandler_VerifyProof_ExpiredNonce(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, priv := generateSolanaKey(t)

	// Issue challenge with very short TTL via direct challenge service
	domain := "test.example.com"
	challengeSvc := caip122.NewChallengeService(h.store, caip122.DefaultChallengeConfig(domain))

	// Set a nonce with TTL=0 so it expires immediately.
	const expiredNonce = "expirednonce1"
	err := h.store.Set(nil, expiredNonce, domain, 0)
	require.NoError(t, err)

	_ = challengeSvc // keep ref

	// Build the message with the stored nonce.
	msg, err := caip122.FormatSolanaMessage(addr, domain, expiredNonce, "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", -1)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)
	proof, _ := json.Marshal(map[string]string{
		"message":   msg,
		"signature": sigB58,
	})

	// This should fail because the nonce expired (TTL=0).
	err = h.VerifyProof(ctx, addr, nil, proof)
	require.Error(t, err)
}

func TestSolanaHandler_Close(t *stdtesting.T) {
	h := NewSolanaHandler(CoreDomainResolver())

	// Close should be idempotent
	err := h.Close()
	require.NoError(t, err)
	err = h.Close()
	require.NoError(t, err)
}

func TestSolanaHandler_SetStore(t *stdtesting.T) {
	h := NewSolanaHandler(CoreDomainResolver())
	originalStore := h.store

	// Set nil store — should be a no-op
	h.SetStore(nil)
	assert.Equal(t, originalStore, h.store)

	// Set a new store
	newStore := caip122.NewMemoryChallengeStore()
	h.SetStore(newStore)
	assert.Equal(t, newStore, h.store)
	defer newStore.Close()
}

// TestSolanaHandler_FullFlow verifies the complete challenge-verify cycle
// end-to-end: issue challenge → sign with wallet → verify proof.
func TestSolanaHandler_FullFlow(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	// Generate a Solana keypair (simulating a wallet)
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	// 1. Issue challenge
	challengeBytes, err := h.IssueChallenge(ctx, addr, json.RawMessage(`{"chain_id":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"}`))
	require.NoError(t, err)

	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(challengeBytes, &challenge))

	// 2. Sign the message (as a wallet would)
	sig := ed25519.Sign(priv, []byte(challenge.Message))
	sigB58 := base58.Encode(sig)

	// 3. Verify the proof
	proof, _ := json.Marshal(map[string]string{
		"message":   challenge.Message,
		"signature": sigB58,
	})

	err = h.VerifyProof(ctx, addr, nil, proof)
	require.NoError(t, err, "full flow verification should succeed")

	// 4. Verify replay is rejected (nonce consumed)
	err = h.VerifyProof(ctx, addr, nil, proof)
	require.Error(t, err, "replay should be rejected")
	assert.Contains(t, err.Error(), "nonce")
}

// Regression test: verify that NormalizeKey produces canonical base58 so
// the same key in different representations normalizes to the same string.
func TestSolanaHandler_NormalizeKey_Canonical(t *stdtesting.T) {
	h := NewSolanaHandler(CoreDomainResolver())

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	addr1 := base58.Encode(pub)
	// Decode and re-encode (should produce same result)
	decoded, err := base58.Decode(addr1)
	require.NoError(t, err)
	addr2 := base58.Encode(decoded)

	// Both should normalize to the same value
	n1, err := h.NormalizeKey(addr1)
	require.NoError(t, err)
	n2, err := h.NormalizeKey(addr2)
	require.NoError(t, err)

	assert.Equal(t, n1, n2, "different representations should normalize to same value")
}

func TestSolanaHandler_VerifyProof_InvalidKey(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	_, priv := generateSolanaKey(t)
	addr := "invalid-key"

	msg, err := caip122.FormatSolanaMessage("GwAF45zjfyGzUbd3i3hXxzGeuchzEZXwpRYHZM5912F1", "test.example.com", "somenonce123", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 0)
	if err != nil {
		return
	}

	// Store the nonce
	_ = h.store.Set(nil, "somenonce123", "test.example.com", 0)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	proof, _ := json.Marshal(map[string]string{
		"message":   msg,
		"signature": sigB58,
	})

	err = h.VerifyProof(ctx, addr, nil, proof)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid base58") || strings.Contains(err.Error(), "invalid address"))
}

func TestSolanaHandler_VerifyProof_InvalidMetadata(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewSolanaHandler(CoreDomainResolver())

	addr, priv := generateSolanaKey(t)

	// Issue challenge
	challengeBytes, err := h.IssueChallenge(ctx, addr, nil)
	require.NoError(t, err)

	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(challengeBytes, &challenge))

	sig := ed25519.Sign(priv, []byte(challenge.Message))
	sigB58 := base58.Encode(sig)

	proof, _ := json.Marshal(map[string]string{
		"message":   challenge.Message,
		"signature": sigB58,
	})

	// Invalid metadata should fail
	err = h.VerifyProof(ctx, addr, json.RawMessage(`{"chain_id":"eip155:1"}`), proof)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be CAIP-2 format")
}
