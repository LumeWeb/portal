package caip122

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSolanaMessage_Valid(t *testing.T) {
	addr := "GwAF45zjfyGzUbd3i3hXxzGeuchzEZXwpRYHZM5912F1"
	domain := "service.org"
	nonce := "32891757"
	chainID := "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

	msg, err := FormatSolanaMessage(addr, domain, nonce, chainID, 5*time.Minute)
	require.NoError(t, err)

	assert.Contains(t, msg, "service.org wants you to sign in with your Solana account:")
	assert.Contains(t, msg, addr)
	assert.Contains(t, msg, "URI: https://service.org")
	assert.Contains(t, msg, "Version: 1")
	assert.Contains(t, msg, "Nonce: 32891757")
	assert.Contains(t, msg, "Chain ID: 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp")
	assert.Contains(t, msg, "Expiration Time:")
}

func TestFormatSolanaMessage_InvalidChainID(t *testing.T) {
	_, err := FormatSolanaMessage("GwAF45zjfyGzUbd3i3hXxzGeuchzEZXwpRYHZM5912F1", "service.org", "nonce", "eip155:1", 5*time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be CAIP-2 format (solana:<genesis>)")
}

func TestParseSolanaMessage_RoundTrip(t *testing.T) {
	addr := "GwAF45zjfyGzUbd3i3hXxzGeuchzEZXwpRYHZM5912F1"
	domain := "service.org"
	nonce := "32891757"
	chainID := "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

	msg, err := FormatSolanaMessage(addr, domain, nonce, chainID, 5*time.Minute)
	require.NoError(t, err)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	assert.Equal(t, domain, parsed.GetDomain())
	assert.Equal(t, addr, parsed.GetAddress())
	assert.Equal(t, nonce, parsed.GetNonce())
	assert.Equal(t, "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", parsed.GetChainID())
}

func TestParseSolanaMessage_MissingHeader(t *testing.T) {
	_, err := ParseSolanaMessage("invalid message\n0x1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be parsed")
}

func TestParseSolanaMessage_MissingAddress(t *testing.T) {
	_, err := ParseSolanaMessage("service.org wants you to sign in with your Solana account:")
	require.Error(t, err)
}

func TestSolanaMessage_VerifyEd25519_Success(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)
	domain := "service.org"
	nonce := "testnonce123"
	chainID := "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

	msg, err := FormatSolanaMessage(addr, domain, nonce, chainID, 5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	verifiedAddr, err := parsed.verifyEd25519(sigB58)
	require.NoError(t, err)
	assert.Equal(t, addr, verifiedAddr)
}

func TestSolanaMessage_VerifyEd25519_WrongKey(t *testing.T) {
	pub1, _, _ := ed25519.GenerateKey(nil)
	addr := base58.Encode(pub1)
	domain := "service.org"
	nonce := "testnonce123"
	chainID := "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

	msg, err := FormatSolanaMessage(addr, domain, nonce, chainID, 5*time.Minute)
	require.NoError(t, err)

	// Sign with a different key
	_, priv2, _ := ed25519.GenerateKey(nil)
	sig := ed25519.Sign(priv2, []byte(msg))
	sigB58 := base58.Encode(sig)

	// Parse with addr1 but verify with sig from key2
	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	_, err = parsed.verifyEd25519(sigB58)
	require.Error(t, err)
	// The address in the message is addr1 (from pub1), but the signature
	// was created with priv2. Verification should fail.
}

func TestSolanaMessage_Verify_Expired(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	// Create a message that's already expired
	msg, err := FormatSolanaMessage(addr, "service.org", "testnonce1234", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", -5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	domain := "service.org"
	nonce := "testnoncex"
	_, err = parsed.Verify(sigB58, &domain, &nonce, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestSolanaMessage_Verify_DomainMismatch(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	msg, err := FormatSolanaMessage(addr, "service.org", "testnoncex", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	wrongDomain := "evil.com"
	nonce := "testnoncex"
	_, err = parsed.Verify(sigB58, &wrongDomain, &nonce, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "domain")
}

func TestSolanaMessage_Verify_NonceMismatch(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	msg, err := FormatSolanaMessage(addr, "service.org", "correctnonce", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	domain := "service.org"
	wrongNonce := "wrongnonce"
	_, err = parsed.Verify(sigB58, &domain, &wrongNonce, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonce")
}

func TestSolanaMessage_VerifyEd25519_InvalidBase58Sig(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	msg, err := FormatSolanaMessage(addr, "service.org", "testnoncex", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 5*time.Minute)
	require.NoError(t, err)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	_, err = parsed.verifyEd25519("!!!invalid-base58!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base58")
}

func TestSolanaMessage_VerifyEd25519_InvalidSigLength(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	msg, err := FormatSolanaMessage(addr, "service.org", "testnoncex", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 5*time.Minute)
	require.NoError(t, err)

	parsed, err := ParseSolanaMessage(msg)
	require.NoError(t, err)

	// 32 bytes instead of 64
	shortSig := base58.Encode(make([]byte, 32))
	_, err = parsed.verifyEd25519(shortSig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature length")
}

func TestVerifySolanaChallenge_Success(t *testing.T) {
	store := NewMemoryChallengeStore()
	defer store.Close()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)
	domain := "service.org"
	nonce := "testverifynonce"
	chainID := "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"

	// Store the nonce
	err = store.Set(nil, nonce, domain, 5*time.Minute)
	require.NoError(t, err)

	msg, err := FormatSolanaMessage(addr, domain, nonce, chainID, 5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	verified, err := VerifySolanaChallenge(nil, store, msg, sigB58)
	require.NoError(t, err)
	assert.Equal(t, addr, verified)
}

func TestVerifySolanaChallenge_InvalidNonce(t *testing.T) {
	store := NewMemoryChallengeStore()
	defer store.Close()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	addr := base58.Encode(pub)

	msg, err := FormatSolanaMessage(addr, "service.org", "neverstored1", "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", 5*time.Minute)
	require.NoError(t, err)

	sig := ed25519.Sign(priv, []byte(msg))
	sigB58 := base58.Encode(sig)

	_, err = VerifySolanaChallenge(nil, store, msg, sigB58)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired nonce")
}

func TestEncodeSolanaChallengeResponse(t *testing.T) {
	data, err := EncodeSolanaChallengeResponse("mynonce", "mymessage")
	require.NoError(t, err)

	var resp struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	err = json.Unmarshal(data, &resp)
	require.NoError(t, err)
	assert.Equal(t, "mynonce", resp.Nonce)
	assert.Equal(t, "mymessage", resp.Message)
}
