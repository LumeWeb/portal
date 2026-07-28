package caip122

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/relvacode/iso8601"
)

// --- SIWS message regex (mirrors the EIP-4361 regex pattern in message.go) ---
//
// The Solana CAIP-122 profile uses the same field layout as EIP-4361 but with:
//   - "Solana account" instead of "Ethereum account"
//   - Base58 address (32-byte Ed25519 public key) instead of 0x hex
//   - Chain ID as raw genesis hash string instead of integer

const (
	_siwsDomain   = `(?P<domain>([^/?#]+)) wants you to sign in with your Solana account:\n`
	_siwsAddress  = `(?P<address>[1-9A-HJ-NP-Za-km-z]+)\n`
	_siwsStatement = `((?P<statement>[^\n]+)\n)?\n`
	_siwsURILine  = `URI: (?P<uri>[^\n]+)\n`
	_siwsVersion  = `Version: (?P<version>1)\n`
	_siwsChainID  = `Chain ID: (?P<chainId>[^\n]+)\n`
	_siwsNonce    = `Nonce: (?P<nonce>[a-zA-Z0-9]{8,})\n`
	_siwsIssuedAt = `Issued At: (?P<issuedAt>[^\n]+)\n`
	_siwsExpiration = `(?:Expiration Time: (?P<expirationTime>[^\n]+)\n)?`
	_siwsNotBefore = `(?:Not Before: (?P<notBefore>[^\n]+)\n)?`
	_siwsRequestID = `(?:Request ID: (?P<requestId>[^\n]+)\n)?`
	_siwsResources = `(?:Resources:\n(?P<resources>(?:- [^\n]+\n?)*))?`
)

var _SIWS_MESSAGE = regexp.MustCompile(fmt.Sprintf(`^%s%s%s%s%s%s%s%s%s%s%s%s$`,
	_siwsDomain,
	_siwsAddress,
	_siwsStatement,
	_siwsURILine,
	_siwsVersion,
	_siwsChainID,
	_siwsNonce,
	_siwsIssuedAt,
	_siwsExpiration,
	_siwsNotBefore,
	_siwsRequestID,
	_siwsResources,
))

// SolanaMessage represents a parsed CAIP-122 Solana namespace sign-in message.
// Unlike Ethereum's EIP-4361, the Solana profile uses:
//   - Ed25519 signatures (not secp256k1 recovery)
//   - Base58-encoded addresses (32-byte public keys)
//   - Chain ID as a raw genesis hash string (CAIP-2 solana:<hash> stored in metadata)
type SolanaMessage struct {
	domain         string
	address        string
	uri            string
	statement      *string
	nonce          string
	version        string
	chainID        string
	issuedAt       string
	expirationTime *string
	notBefore      *string
	requestID      *string
	resources      []string
}

// FormatSolanaMessage constructs a CAIP-122 Solana sign-in message for the
// client to sign with their Solana wallet. The chainID must be a CAIP-2
// identifier in the form "solana:<genesis_hash>".
func FormatSolanaMessage(address, domain, nonce, chainID string, ttl time.Duration) (string, error) {
	if !strings.HasPrefix(chainID, "solana:") {
		return "", fmt.Errorf("caip122/solana: chain_id must be CAIP-2 format (solana:<genesis>), got %q", chainID)
	}
	if genesisHash := strings.TrimPrefix(chainID, "solana:"); genesisHash == "" {
		return "", fmt.Errorf("caip122/solana: chain_id must be CAIP-2 format (solana:<genesis>), got %q", chainID)
	}
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9]{8,}$`, nonce); !matched {
		return "", fmt.Errorf("caip122/solana: nonce must be at least 8 alphanumeric characters")
	}
	addrBytes, err := base58.Decode(address)
	if err != nil {
		return "", fmt.Errorf("caip122/solana: invalid base58 address: %w", err)
	}
	if len(addrBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("caip122/solana: address must decode to %d bytes, got %d", ed25519.PublicKeySize, len(addrBytes))
	}
	genesisHash := strings.TrimPrefix(chainID, "solana:")

	uri := "https://" + domain
	now := time.Now().UTC()
	issuedAt := now.Format(time.RFC3339)
	expiration := now.Add(ttl).Format(time.RFC3339)

	return fmt.Sprintf(`%s wants you to sign in with your Solana account:
%s

URI: %s
Version: 1
Chain ID: %s
Nonce: %s
Issued At: %s
Expiration Time: %s
`, domain, address, uri, genesisHash, nonce, issuedAt, expiration), nil
}

// ParseSolanaMessage parses a CAIP-122 Solana sign-in message string using
// a compiled regex with named capture groups, mirroring the EIP-4361 parser.
func ParseSolanaMessage(message string) (*SolanaMessage, error) {
	match := _SIWS_MESSAGE.FindStringSubmatch(message)
	if match == nil {
		return nil, &InvalidMessage{"Message could not be parsed"}
	}

	result := make(map[string]string)
	for i, name := range _SIWS_MESSAGE.SubexpNames() {
		if i != 0 && name != "" && match[i] != "" {
			result[name] = match[i]
		}
	}

	m := &SolanaMessage{
		domain:   result["domain"],
		address:  result["address"],
		uri:      result["uri"],
		version:  result["version"],
		chainID:  result["chainId"],
		nonce:    result["nonce"],
		issuedAt: result["issuedAt"],
	}

	if s, ok := result["statement"]; ok {
		m.statement = &s
	}
	if s, ok := result["expirationTime"]; ok {
		m.expirationTime = &s
	}
	if s, ok := result["notBefore"]; ok {
		m.notBefore = &s
	}
	if s, ok := result["requestId"]; ok {
		m.requestID = &s
	}
	if resources, ok := result["resources"]; ok && resources != "" {
		for _, line := range strings.Split(strings.TrimRight(resources, "\n"), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				m.resources = append(m.resources, strings.TrimPrefix(line, "- "))
			}
		}
	}

	if m.domain == "" {
		return nil, &InvalidMessage{"`domain` must not be empty"}
	}
	if m.address == "" {
		return nil, &InvalidMessage{"`address` must not be empty"}
	}
	// Validate the address decodes to a 32-byte Ed25519 public key.
	addrBytes, err := base58.Decode(m.address)
	if err != nil {
		return nil, &InvalidMessage{fmt.Sprintf("`address` is not valid base58: %s", err)}
	}
	if len(addrBytes) != ed25519.PublicKeySize {
		return nil, &InvalidMessage{fmt.Sprintf("`address` must decode to %d bytes, got %d", ed25519.PublicKeySize, len(addrBytes))}
	}
	if m.chainID == "" {
		return nil, &InvalidMessage{"`chainId` must not be empty"}
	}
	if m.nonce == "" {
		return nil, &InvalidMessage{"`nonce` must not be empty"}
	}
	if m.issuedAt == "" {
		return nil, &InvalidMessage{"`issuedAt` must not be empty"}
	}

	return m, nil
}

// GetDomain returns the domain from the parsed message.
func (m *SolanaMessage) GetDomain() string { return m.domain }

// GetNonce returns the nonce from the parsed message.
func (m *SolanaMessage) GetNonce() string { return m.nonce }

// GetAddress returns the base58-encoded Solana address from the parsed message.
func (m *SolanaMessage) GetAddress() string { return m.address }

// GetChainID returns the raw genesis hash from the parsed message's Chain ID field.
func (m *SolanaMessage) GetChainID() string { return m.chainID }

// ValidNow checks if the message is valid at the current time.
func (m *SolanaMessage) ValidNow() (bool, error) {
	return m.ValidAt(time.Now().UTC())
}

// ValidAt checks if the message is valid at the given time.
func (m *SolanaMessage) ValidAt(t time.Time) (bool, error) {
	if m.issuedAt != "" {
		issued, err := iso8601.ParseString(m.issuedAt)
		if err != nil {
			return false, fmt.Errorf("invalid issuedAt: %w", err)
		}
		if t.Before(issued) {
			return false, errors.New("message issued in the future")
		}
	}
	if m.expirationTime != nil {
		exp, err := iso8601.ParseString(*m.expirationTime)
		if err != nil {
			return false, fmt.Errorf("invalid expirationTime: %w", err)
		}
		if t.After(exp) {
			return false, errors.New("message has expired")
		}
	}
	if m.notBefore != nil {
		nb, err := iso8601.ParseString(*m.notBefore)
		if err != nil {
			return false, fmt.Errorf("invalid notBefore: %w", err)
		}
		if t.Before(nb) {
			return false, errors.New("message not yet valid")
		}
	}
	return true, nil
}

// Verify validates the Solana SIWS message signature using Ed25519.
// The signature must be base58-encoded raw 64-byte Ed25519 signature.
// Returns the verified address (base58) on success.
func (m *SolanaMessage) Verify(signature string, domain *string, nonce *string, timestamp *time.Time) (string, error) {
	var err error
	if timestamp != nil {
		_, err = m.ValidAt(*timestamp)
	} else {
		_, err = m.ValidNow()
	}
	if err != nil {
		return "", err
	}

	if domain != nil && m.GetDomain() != *domain {
		return "", &InvalidSignature{"Message domain doesn't match"}
	}

	if nonce != nil && m.GetNonce() != *nonce {
		return "", &InvalidSignature{"Message nonce doesn't match"}
	}

	return m.verifyEd25519(signature)
}

// verifyEd25519 verifies the base58-encoded Ed25519 signature against the
// base58-encoded public key (address) and the original message text.
func (m *SolanaMessage) verifyEd25519(signature string) (string, error) {
	sigBytes, err := base58.Decode(signature)
	if err != nil {
		return "", fmt.Errorf("caip122/solana: invalid base58 signature: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return "", fmt.Errorf("caip122/solana: invalid signature length: expected %d bytes, got %d", ed25519.SignatureSize, len(sigBytes))
	}

	pubKey, err := base58.Decode(m.address)
	if err != nil {
		return "", fmt.Errorf("caip122/solana: invalid base58 address: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("caip122/solana: invalid public key length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKey))
	}

	// The signed payload is the original message text. We reconstruct it
	// from the parsed fields to ensure the signed content matches what we
	// validated (not a tampered version).
	messageText := m.serialize()
	if !ed25519.Verify(ed25519.PublicKey(pubKey), []byte(messageText), sigBytes) {
		return "", &InvalidSignature{"Ed25519 signature verification failed"}
	}

	return m.address, nil
}

// serialize reconstructs the canonical message text that was signed.
// This must match FormatSolanaMessage output exactly — the regex parser
// guarantees the fields came from a structurally valid message, and this
// ensures the signed bytes match what we parsed.
func (m *SolanaMessage) serialize() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s wants you to sign in with your Solana account:\n", m.domain)
	sb.WriteString(m.address)
	if m.statement != nil {
		sb.WriteString("\n")
		sb.WriteString(*m.statement)
	}
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "URI: %s\n", m.uri)
	fmt.Fprintf(&sb, "Version: %s\n", m.version)
	fmt.Fprintf(&sb, "Chain ID: %s\n", m.chainID)
	fmt.Fprintf(&sb, "Nonce: %s\n", m.nonce)
	fmt.Fprintf(&sb, "Issued At: %s\n", m.issuedAt)
	if m.expirationTime != nil {
		fmt.Fprintf(&sb, "Expiration Time: %s\n", *m.expirationTime)
	}
	if m.notBefore != nil {
		fmt.Fprintf(&sb, "Not Before: %s\n", *m.notBefore)
	}
	if m.requestID != nil {
		fmt.Fprintf(&sb, "Request ID: %s\n", *m.requestID)
	}
	if len(m.resources) > 0 {
		sb.WriteString("Resources:\n")
		for _, r := range m.resources {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}
	return sb.String()
}

// VerifySolanaChallenge is a convenience function that parses a Solana SIWS
// message, validates the nonce via the challenge store, and verifies the
// Ed25519 signature. Returns the verified address (base58).
// The nonce is consumed atomically via store.Take before signature
// verification to prevent concurrent replay attacks (TOCTOU).
func VerifySolanaChallenge(ctx context.Context, store ChallengeStore, message string, signature string) (string, error) {
	msg, err := ParseSolanaMessage(message)
	if err != nil {
		return "", fmt.Errorf("caip122/solana: invalid message: %w", err)
	}
	return VerifySolanaChallengeParsed(ctx, store, msg, signature)
}

// VerifySolanaChallengeParsed verifies a pre-parsed Solana SIWS message.
// This avoids re-parsing when the caller already has the parsed message
// (e.g., for chain_id validation before verification).
func VerifySolanaChallengeParsed(ctx context.Context, store ChallengeStore, msg *SolanaMessage, signature string) (string, error) {
	// Atomically consume the nonce before verification. This prevents
	// concurrent replay: two requests with the same valid proof cannot
	// both pass, because only one Take succeeds. If verification fails
	// the nonce is already consumed — the user burned their challenge by
	// submitting an invalid proof, matching the Ethereum handler pattern.
	storedDomain, found, err := store.Take(ctx, msg.GetNonce())
	if err != nil {
		return "", fmt.Errorf("caip122/solana: nonce lookup failed: %w", err)
	}
	if !found {
		return "", errors.New("caip122/solana: invalid or expired nonce")
	}

	domain := &storedDomain
	nonce := msg.GetNonce()
	address, err := msg.Verify(signature, domain, &nonce, nil)
	if err != nil {
		return "", err
	}

	return address, nil
}

// EncodeSolanaChallengeResponse marshals the nonce and message into a JSON
// payload identical to the Ethereum handler's response format.
func EncodeSolanaChallengeResponse(nonce string, message string) ([]byte, error) {
	return json.Marshal(struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}{
		Nonce:   nonce,
		Message: message,
	})
}
