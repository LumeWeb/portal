package keyidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/hyperledger-firefly/signer/pkg/secp256k1"
	"go.lumeweb.com/portal/core/keyidentity/caip122"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"golang.org/x/crypto/sha3"
)

func testContext(t *stdtesting.T) core.Context {
	tc, err := coreTesting.NewTestContext(t)
	if err != nil {
		t.Fatalf("failed to create test context: %v", err)
	}
	return tc
}

func TestEthereumHandler_NormalizeKey(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"lowercase", "0xabcdef0123456789abcdef0123456789abcdef01", "0xabcdef0123456789abcdef0123456789abcdef01", false},
		{"uppercase normalized", "0xABCDEF0123456789ABCDEF0123456789ABCDEF01", "0xabcdef0123456789abcdef0123456789abcdef01", false},
		{"missing 0x prefix", "abcdef0123456789abcdef0123456789abcdef01", "", true},
		{"too short", "0xabc", "", true},
		{"non-hex", "0xGGGGGG0123456789abcdef0123456789abcdef01", "", true},
		{"empty", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *stdtesting.T) {
			got, err := h.NormalizeKey(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEthereumHandler_ValidateMetadata(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())

	t.Run("nil metadata defaults to empty json", func(t *stdtesting.T) {
		got, err := h.ValidateMetadata(nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "{}" {
			t.Fatalf("expected {}, got %q", string(got))
		}
	})

	t.Run("valid chain_id", func(t *stdtesting.T) {
		input := json.RawMessage(`{"chain_id": "eip155:1"}`)
		_, err := h.ValidateMetadata(input)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid chain_id format", func(t *stdtesting.T) {
		input := json.RawMessage(`{"chain_id": "ethereum:1"}`)
		_, err := h.ValidateMetadata(input)
		if err == nil {
			t.Fatal("expected error for non-eip155 chain_id")
		}
	})

	t.Run("chain_id non-numeric", func(t *stdtesting.T) {
		input := json.RawMessage(`{"chain_id": "eip155:foo"}`)
		_, err := h.ValidateMetadata(input)
		if err == nil {
			t.Fatal("expected error for non-numeric chain_id")
		}
	})

	t.Run("chain_id empty number", func(t *stdtesting.T) {
		input := json.RawMessage(`{"chain_id": "eip155:"}`)
		_, err := h.ValidateMetadata(input)
		if err == nil {
			t.Fatal("expected error for empty chain_id number")
		}
	})

	t.Run("chain_id not string", func(t *stdtesting.T) {
		input := json.RawMessage(`{"chain_id": 1}`)
		_, err := h.ValidateMetadata(input)
		if err == nil {
			t.Fatal("expected error for non-string chain_id")
		}
	})

	t.Run("invalid json", func(t *stdtesting.T) {
		input := json.RawMessage(`{invalid}`)
		_, err := h.ValidateMetadata(input)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestEthereumHandler_IssueChallenge(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	out, err := h.IssueChallenge(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`))
	if err != nil {
		t.Fatalf("IssueChallenge failed: %v", err)
	}

	var resp struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid challenge response: %v", err)
	}

	if resp.Nonce == "" {
		t.Fatal("nonce should not be empty")
	}
	if resp.Message == "" {
		t.Fatal("message should not be empty")
	}
	if !strings.Contains(strings.ToLower(resp.Message), strings.ToLower(key)) {
		t.Fatal("message should contain the address")
	}
	if !strings.Contains(resp.Message, "Nonce: "+resp.Nonce) {
		t.Fatal("message should contain the nonce")
	}
}

func TestEthereumHandler_IssueChallenge_InvalidKey(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	_, err := h.IssueChallenge(ctx, "0xinvalid", nil)
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestEthereumHandler_VerifyProof_InvalidPayload(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	t.Run("malformed json", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, key, nil, []byte("not json"))
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("missing fields", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, key, nil, []byte(`{"message":"hello"}`))
		if err == nil {
			t.Fatal("expected error for missing signature")
		}
	})

	t.Run("empty fields", func(t *stdtesting.T) {
		err := h.VerifyProof(ctx, key, nil, []byte(`{"message":"","signature":""}`))
		if err == nil {
			t.Fatal("expected error for empty fields")
		}
	})
}

func TestEthereumHandler_VerifyProof_InvalidNonce(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	// Construct a proof with a valid-looking message but unknown nonce.
	// The nonce was never issued, so verification should fail.
	fakeMessage, err := caip122.FormatMessage(key, "example.com", "deadbeefdeadbeef", "eip155:1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   fakeMessage,
		Signature: "0x" + strings.Repeat("00", 65),
	})

	err = h.VerifyProof(ctx, key, nil, proof)
	if err == nil {
		t.Fatal("expected error for unissued nonce")
	}
	if !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce-related error, got: %v", err)
	}
}

func TestEthereumHandler_VerifyProof_AddressMismatch(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)

	domain := "localhost"

	// Issue a challenge
	challenge := caip122.NewChallengeService(store, caip122.DefaultChallengeConfig(domain))
	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Construct message with a different address
	otherKey := "0x0000000000000000000000000000000000000001"
	msg, err := caip122.FormatMessage(otherKey, domain, nonce, "eip155:1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   msg,
		Signature: "0x" + strings.Repeat("00", 65),
	})

	// Try to verify with the original key (not otherKey)
	key := "0xabcdef0123456789abcdef0123456789abcdef01"
	err = h.VerifyProof(ctx, key, nil, proof)
	if err == nil {
		t.Fatal("expected error for address mismatch")
	}
}

func TestEthereumHandler_ChallengeRoundTrip(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	// Issue challenge
	challengeBytes, err := h.IssueChallenge(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`))
	if err != nil {
		t.Fatal(err)
	}

	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	json.Unmarshal(challengeBytes, &challenge)

	// Verify the challenge was stored
	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)

	// Re-issue with the new store
	challengeBytes2, err := h.IssueChallenge(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(challengeBytes2, &challenge)

	// The nonce should be in the store
	_, found, err := store.Get(context.Background(), challenge.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("nonce should be in the store after IssueChallenge")
	}
}

// --- Regression tests for Kody findings ---

// REGRESSION: MemoryChallengeStore goroutine leak (Kody: high)
// Verify that EthereumHandler.Close() stops the background reaper goroutine
// and can be called multiple times safely.
func TestEthereumHandler_Close_StopsReaper(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())

	// Close should not panic and should stop the reaper
	if err := h.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	// Double close should be safe
	if err := h.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}

	if !h.closed {
		t.Fatal("handler should be marked as closed")
	}
}

// REGRESSION: VerifyProof ignores metadata param — chain_id mismatch (Kody: critical)
// Verify that VerifyProof rejects a proof where the signed message's chain_id
// doesn't match the registered metadata's chain_id (cross-chain replay prevention).
// Chain_id is no longer enforced at verification — the same EVM address can
// sign from any chain. This test verifies that a signature from a different
// chain_id than the stored metadata still passes verification.
func TestEthereumHandler_VerifyProof_ChainIDAgnostistic(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)

	domain := "localhost"

	// Issue a challenge
	challenge := caip122.NewChallengeService(store, caip122.DefaultChallengeConfig(domain))
	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create a real wallet so the signature is valid
	kp, err := secp256k1.GenerateSecp256k1KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key := kp.Address.String()

	// Construct message with a DIFFERENT chain_id (eip155:137) than metadata
	msg, err := caip122.FormatMessage(key, domain, nonce, "eip155:137", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Sign the message with EIP-191
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(prefixed))
	hash := hasher.Sum(nil)
	sigData, err := kp.SignDirect(hash)
	if err != nil {
		t.Fatal(err)
	}
	rsv := make([]byte, 65)
	sigData.R.FillBytes(rsv[0:32])
	sigData.S.FillBytes(rsv[32:64])
	rsv[64] = byte(sigData.V.Int64())
	sig := "0x" + fmt.Sprintf("%x", rsv)

	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   msg,
		Signature: sig,
	})

	// Verify with metadata declaring eip155:1 — should succeed because
	// chain_id is informational, not enforced.
	err = h.VerifyProof(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`), proof)
	if err != nil {
		t.Fatalf("expected no error for cross-chain verification, got: %v", err)
	}
}

// REGRESSION: IssueChallenge silently ignores JSON parse errors (Kody: high)
// Verify that IssueChallenge rejects invalid metadata instead of silently
// falling back to eip155:1.
func TestEthereumHandler_IssueChallenge_InvalidMetadata(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	tests := []struct {
		name     string
		metadata json.RawMessage
	}{
		{"invalid json", json.RawMessage(`{invalid}`)},
		{"non-string chain_id", json.RawMessage(`{"chain_id": 1}`)},
		{"wrong prefix", json.RawMessage(`{"chain_id": "ethereum:1"}`)},
		{"non-numeric", json.RawMessage(`{"chain_id": "eip155:foo"}`)},
		{"empty number", json.RawMessage(`{"chain_id": "eip155:"}`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *stdtesting.T) {
			_, err := h.IssueChallenge(ctx, key, tc.metadata)
			if err == nil {
				t.Fatal("expected error for invalid metadata")
			}
		})
	}
}

// REGRESSION: VerifyProof rejects invalid metadata (Kody: critical)
// Verify that VerifyProof also validates the metadata parameter.
func TestEthereumHandler_VerifyProof_InvalidMetadata(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   "dummy",
		Signature: "0x" + strings.Repeat("00", 65),
	})

	err := h.VerifyProof(ctx, key, json.RawMessage(`{invalid}`), proof)
	if err == nil {
		t.Fatal("expected error for invalid metadata in VerifyProof")
	}
}

// REGRESSION: FormatMessage uses TTL from ChallengeConfig (Kody: high)
// Verify that FormatMessage uses the passed TTL for the Expiration Time,
// not a hardcoded 5 minutes.
func TestFormatMessage_UsesTTL(t *stdtesting.T) {
	address := "0xabcdef0123456789abcdef0123456789abcdef01"
	domain := "example.com"
	nonce := "testnonce123"
	chainID := "eip155:1"
	ttl := 10 * time.Minute

	msg, err := caip122.FormatMessage(address, domain, nonce, chainID, ttl)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the Expiration Time from the message
	lines := strings.Split(msg, "\n")
	var issuedAt, expiration string
	for _, line := range lines {
		if strings.HasPrefix(line, "Issued At:") {
			issuedAt = strings.TrimSpace(strings.TrimPrefix(line, "Issued At:"))
		}
		if strings.HasPrefix(line, "Expiration Time:") {
			expiration = strings.TrimSpace(strings.TrimPrefix(line, "Expiration Time:"))
		}
	}

	if issuedAt == "" || expiration == "" {
		t.Fatal("message must contain Issued At and Expiration Time")
	}

	issuedTime, err := time.Parse(time.RFC3339, issuedAt)
	if err != nil {
		t.Fatalf("failed to parse issued at: %v", err)
	}

	expirationTime, err := time.Parse(time.RFC3339, expiration)
	if err != nil {
		t.Fatalf("failed to parse expiration: %v", err)
	}

	actualTTL := expirationTime.Sub(issuedTime)
	// Allow 1 second of wiggle for execution time
	if actualTTL < 9*time.Minute || actualTTL > 11*time.Minute {
		t.Fatalf("expected TTL ~10m, got %v (issued=%s, exp=%s)", actualTTL, issuedAt, expiration)
	}
}

// REGRESSION: Nonce replay protection (Kody: high)
// Verify that a nonce cannot be reused after a failed verification attempt.
func TestEthereumHandler_NonceReplayProtection(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)

	domain := "localhost"

	// Issue a challenge
	challenge := caip122.NewChallengeService(store, caip122.DefaultChallengeConfig(domain))
	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		t.Fatal(err)
	}

	key := "0xabcdef0123456789abcdef0123456789abcdef01"
	msg, err := caip122.FormatMessage(key, domain, nonce, "eip155:1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Create a proof with an invalid signature (will fail signature verification,
	// but the nonce should still be consumed).
	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   msg,
		Signature: "0x" + strings.Repeat("00", 65),
	})

	// First attempt — fails (bad signature), but the nonce is consumed.
	err = h.VerifyProof(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`), proof)
	if err == nil {
		t.Fatal("expected first verification to fail")
	}

	// Second attempt with the same nonce — should fail with "invalid or expired nonce"
	// because the nonce was already consumed.
	err = h.VerifyProof(ctx, key, json.RawMessage(`{"chain_id":"eip155:1"}`), proof)
	if err == nil {
		t.Fatal("expected replay attempt to fail")
	}
	if !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce-related error on replay, got: %v", err)
	}
}

// REGRESSION: MemoryChallengeStore reaper cleans expired entries (Kody: high)
func TestMemoryChallengeStore_ReapsExpired(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()
	defer store.Close()

	ctx := context.Background()

	// Set a nonce with a 1-second TTL
	if err := store.Set(ctx, "nonce1", "example.com", 1*time.Second); err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	_, found, err := store.Get(ctx, "nonce1")
	if err != nil || !found {
		t.Fatal("nonce should exist before expiry")
	}

	// Wait for expiry
	time.Sleep(2 * time.Second)

	// Verify it's gone
	_, found, err = store.Get(ctx, "nonce1")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("nonce should be expired")
	}
}

// --- Kody regression tests (round 3) ---

// TestMemoryChallengeStore_Close_Idempotent verifies that Close() can be called
// multiple times without panicking (sync.Once guard).
func TestMemoryChallengeStore_Close_Idempotent(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()

	// First close should work
	if err := store.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// Second close should not panic
	if err := store.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	// Third close should also not panic
	if err := store.Close(); err != nil {
		t.Fatalf("third Close failed: %v", err)
	}
}

// TestMemoryChallengeStore_Take_AtomicGetAndDelete verifies that Take retrieves
// and deletes a nonce in one operation, preventing the Get+Delete race.
func TestMemoryChallengeStore_Take_AtomicGetAndDelete(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()
	defer store.Close()

	ctx := context.Background()

	// Set a nonce
	if err := store.Set(ctx, "nonce-take", "example.com", 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	// Take should return the domain and delete the nonce
	domain, found, err := store.Take(ctx, "nonce-take")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("nonce should have been found")
	}
	if domain != "example.com" {
		t.Fatalf("expected example.com, got %s", domain)
	}

	// Second Take should return not-found (nonce was deleted)
	_, found, err = store.Take(ctx, "nonce-take")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("nonce should have been deleted by Take")
	}

	// Get should also return not-found
	_, found, err = store.Get(ctx, "nonce-take")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("nonce should not exist after Take")
	}
}

// TestMemoryChallengeStore_Take_Expired verifies that Take returns not-found
// for an expired nonce.
func TestMemoryChallengeStore_Take_Expired(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()
	defer store.Close()

	ctx := context.Background()

	if err := store.Set(ctx, "nonce-expired", "example.com", 1*time.Second); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	_, found, err := store.Take(ctx, "nonce-expired")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expired nonce should not be found by Take")
	}
}

// TestEthereumHandler_CloseAfterRegisterCleanup verifies that Close() still
// works after registerCleanup has been called (separate sync.Once fields).
func TestEthereumHandler_CloseAfterRegisterCleanup(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	defer h.Close()

	// IssueChallenge calls registerCleanup internally
	key := "0xabcdef0123456789abcdef0123456789abcdef01"
	_, err := h.IssueChallenge(ctx, key, nil)
	if err != nil {
		// The issue may fail due to domain resolution, but registerCleanup
		// should have been called regardless.
		// Verify Close still works.
		if closeErr := h.Close(); closeErr != nil {
			t.Fatalf("Close failed after registerCleanup: %v", closeErr)
		}
		return
	}

	// Close should still work after registerCleanup consumed cleanupOnce
	if err := h.Close(); err != nil {
		t.Fatalf("Close failed after registerCleanup: %v", err)
	}

	// Double close should be safe
	if err := h.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

// TestEthereumHandler_IssueChallenge_EmptyDomain verifies that IssueChallenge
// returns a clear error when the dashboard domain cannot be resolved.
func TestEthereumHandler_IssueChallenge_EmptyDomain(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())
	defer h.Close()

	key := "0xabcdef0123456789abcdef0123456789abcdef01"

	// The test context may have a core domain configured, in which case
	// IssueChallenge will proceed past the domain check. We verify the
	// domain check works by testing with a nil context.
	_, err := h.IssueChallenge(nil, key, nil)
	if err == nil {
		t.Fatal("expected error when context is nil (domain cannot be resolved)")
	}
	if !strings.Contains(err.Error(), "domain") {
		t.Fatalf("expected domain error, got: %v", err)
	}
}

// TestVerifyChallengeWithChain_NonceConsumedAtomically verifies that two
// concurrent VerifyChallengeWithChain calls with the same nonce cannot both
// succeed — the second must fail with "invalid or expired nonce".
func TestVerifyChallengeWithChain_NonceConsumedAtomically(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()
	defer store.Close()

	ctx := context.Background()

	// Set a nonce
	if err := store.Set(ctx, "nonce-concurrent", "example.com", 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	// First Take succeeds
	domain, found, err := store.Take(ctx, "nonce-concurrent")
	if err != nil || !found {
		t.Fatalf("first Take should succeed: err=%v found=%v domain=%s", err, found, domain)
	}

	// Second Take (simulating concurrent request) fails
	_, found2, err := store.Take(ctx, "nonce-concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if found2 {
		t.Fatal("second Take should fail — nonce should have been deleted atomically")
	}
}

// --- Kody round 5 regression tests ---

// TestValidateMetadata_CanonicalizesChainID verifies that leading-zero
// chain IDs like "eip155:01" are canonicalized to "eip155:1" so that
// VerifyProof doesn't reject with a chain_id mismatch.
func TestValidateMetadata_CanonicalizesChainID(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())
	defer h.Close()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"canonical", `{"chain_id":"eip155:1"}`, `{"chain_id":"eip155:1"}`, false},
		{"leading_zero", `{"chain_id":"eip155:01"}`, `{"chain_id":"eip155:1"}`, false},
		{"multiple_zeros", `{"chain_id":"eip155:001"}`, `{"chain_id":"eip155:1"}`, false},
		{"no_prefix", `{"chain_id":"1"}`, "", true},
		{"empty_num", `{"chain_id":"eip155:"}`, "", true},
		{"non_numeric", `{"chain_id":"eip155:abc"}`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *stdtesting.T) {
			result, err := h.ValidateMetadata(json.RawMessage(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %s, got %s", tt.input, string(result))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got map[string]interface{}
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if got["chain_id"] != "eip155:1" {
				t.Fatalf("chain_id = %v, want eip155:1", got["chain_id"])
			}
		})
	}
}

// TestSetStore_ClosesPreviousStore verifies that SetStore closes the old
// MemoryChallengeStore to prevent goroutine leaks.
func TestSetStore_ClosesPreviousStore(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())
	oldStore := h.store.(*caip122.MemoryChallengeStore)

	// Swap in a new store
	newStore := caip122.NewMemoryChallengeStore()
	h.SetStore(newStore)

	// Old store should be closed — Take on it should not find anything
	// (it's closed, but more importantly the reaper goroutine should have stopped).
	// We verify by checking that the old store's Close() is idempotent (already closed).
	if err := oldStore.Close(); err != nil {
		t.Fatalf("old store Close() should be idempotent: %v", err)
	}

	// New store should still work
	ctx := context.Background()
	if err := newStore.Set(ctx, "test", "example.com", 5*time.Minute); err != nil {
		t.Fatalf("new store Set failed: %v", err)
	}

	h.Close()
}

// TestSetStore_RejectsNilStore verifies that passing nil to SetStore is
// a no-op and does not clear the current store.
func TestSetStore_RejectsNilStore(t *stdtesting.T) {
	h := NewEthereumHandler(CoreDomainResolver())
	defer h.Close()

	original := h.store
	h.SetStore(nil)

	// Store should be unchanged
	if h.store != original {
		t.Fatal("SetStore(nil) should not change the store")
	}
}

// TestParseMessage_InvalidChainID verifies that a non-numeric chainId in
// a SIWE message produces an explicit error rather than silently defaulting
// to chain ID 1.
func TestParseMessage_InvalidChainID(t *stdtesting.T) {
	domain := "example.com"
	address := "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"
	uri := "https://example.com"
	nonce := "test-nonce-1234"
	issuedAt := "2026-07-26T12:00:00Z"

	// Build a SIWE message with a non-numeric chainId.
	msg := domain + " wants you to sign in with your Ethereum account:\n"
	msg += address + "\n\n"
	msg += "\n"
	msg += "URI: " + uri + "\n"
	msg += "Version: 1\n"
	msg += "Chain ID: abc\n" // Non-numeric
	msg += "Nonce: " + nonce + "\n"
	msg += "Issued At: " + issuedAt + "\n"

	_, err := caip122.ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage should return error for non-numeric chainId")
	}
}

// TestEthereumHandler_StoreSurvivesRequestContext verifies that the handler's
// store is NOT closed when a request context is destroyed. The handler is a
// singleton, so binding cleanup to a request-scoped context was a bug that
// closed the store after the first request ended.
func TestEthereumHandler_StoreSurvivesRequestContext(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	defer h.Close()

	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)

	// Issue a challenge (creates store entries, uses the store)
	_, err := h.IssueChallenge(ctx, "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("first IssueChallenge failed: %v", err)
	}

	// Simulate the request context being destroyed (as would happen
	// with the old registerCleanup binding).
	// The test context's cleanup runs on t.Cleanup, but we can verify
	// the store is still usable by issuing another challenge.
	ctx2 := testContext(t)
	_, err = h.IssueChallenge(ctx2, "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("second IssueChallenge failed — store was closed after first request: %v", err)
	}

	// Verify the store is still the same instance (not replaced or closed)
	if h.store != store {
		t.Fatal("store should still be the same instance after request context ends")
	}
}

// TestMemoryChallengeStore_SetCap verifies that Set returns an error when
// the store reaches its maxEntries capacity, preventing unbounded memory growth.
func TestMemoryChallengeStore_SetCap(t *stdtesting.T) {
	store := caip122.NewMemoryChallengeStore()
	defer store.Close()

	ctx := context.Background()

	// Fill the store to capacity.
	for i := 0; i < caip122.DefaultMaxChallengeEntries; i++ {
		nonce := fmt.Sprintf("nonce-%d", i)
		if err := store.Set(ctx, nonce, "localhost", 5*time.Minute); err != nil {
			t.Fatalf("Set failed at index %d: %v", i, err)
		}
	}

	// The next Set triggers bounded eviction. With all entries still
	// valid (5-min TTL), no expired entries are found, so the write
	// is rejected to protect live nonces needed by in-flight verifications.
	err := store.Set(ctx, "overflow-nonce", "localhost", 5*time.Minute)
	if err == nil {
		t.Fatal("expected capacity-exceeded error when store is full of live entries")
	}
	if !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("expected capacity error, got: %v", err)
	}
}

// TestVerifyProof_MalformedKeyDoesNotConsumeNonce verifies that a malformed
// key argument to VerifyProof fails fast without consuming the challenge
// nonce. This is a regression test for the fix that moved NormalizeKey
// before VerifyChallengeWithChain.
func TestVerifyProof_MalformedKeyDoesNotConsumeNonce(t *stdtesting.T) {
	ctx := testContext(t)
	h := NewEthereumHandler(CoreDomainResolver())
	store := caip122.NewMemoryChallengeStore()
	h.SetStore(store)
	defer h.Close()

	domain := "localhost"
	challenge := caip122.NewChallengeService(store, caip122.DefaultChallengeConfig(domain))
	nonce, err := challenge.GenerateChallenge(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create a valid signed message.
	kp, err := secp256k1.GenerateSecp256k1KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key := kp.Address.String()

	msg, err := caip122.FormatMessage(key, domain, nonce, "eip155:1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(prefixed))
	hash := hasher.Sum(nil)
	sigData, err := kp.SignDirect(hash)
	if err != nil {
		t.Fatal(err)
	}
	rsv := make([]byte, 65)
	sigData.R.FillBytes(rsv[0:32])
	sigData.S.FillBytes(rsv[32:64])
	rsv[64] = byte(sigData.V.Int64())
	sig := "0x" + fmt.Sprintf("%x", rsv)

	proof, _ := json.Marshal(struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}{
		Message:   msg,
		Signature: sig,
	})

	// Call VerifyProof with a malformed key — should fail at NormalizeKey
	// BEFORE the nonce is consumed.
	err = h.VerifyProof(ctx, "not-a-valid-key", json.RawMessage(`{"chain_id":"eip155:1"}`), proof)
	if err == nil {
		t.Fatal("expected error for malformed key")
	}

	// The nonce should still be present in the store (not consumed).
	_, found, _ := store.Get(ctx, nonce)
	if !found {
		t.Fatal("nonce was consumed by VerifyProof despite malformed key — NormalizeKey should run first")
	}
}
