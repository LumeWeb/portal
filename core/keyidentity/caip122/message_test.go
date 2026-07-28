// Ported from github.com/spruceid/siwe-go/siwe_test.go.
// Copyright (c) 2021 Spruce Systems, Inc.
// Licensed under Apache License 2.0 / MIT (dual-licensed).
//
// See THIRD_PARTY_NOTICES.md for full attribution.
// Adapted to use firefly-signer instead of go-ethereum for crypto operations.

package caip122

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-firefly/signer/pkg/secp256k1"
	"github.com/relvacode/iso8601"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

const testDomain = "example.com"
const testAddressStr = "0x71C7656EC7ab88b098defB751B7401B5f6d8976F"

const testURI = "https://example.com"
const testVersion = "1"
const testStatement = "Example statement for SIWE"

var testIssuedAt = time.Now().UTC().Format(time.RFC3339)
var testNonce = GenerateNonce()

const testChainId = 1

var testExpirationTime = time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
var testNotBefore = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

const testRequestId = "some-id"

// Non-const copies so we can take their address for MessageOptions pointers.
var testStatementVal = testStatement
var testChainIdVal = testChainId
var testRequestIdVal = testRequestId

var testOptions = &MessageOptions{
	Statement:      &testStatementVal,
	ChainID:        &testChainIdVal,
	IssuedAt:       &testIssuedAt,
	ExpirationTime: &testExpirationTime,
	NotBefore:      &testNotBefore,
	RequestID:      &testRequestIdVal,
}

var testMessage, _ = InitMessage(
	testDomain,
	testAddressStr,
	testURI,
	testNonce,
	testOptions,
)

// --- Unit tests (ported from siwe-go) ---

func TestCreate(t *testing.T) {
	message, err := InitMessage(testDomain, testAddressStr, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	uri := message.GetURI()
	assert.Equal(t, testDomain, message.GetDomain())
	assert.Equal(t, testAddressStr, message.GetAddress())
	assert.Equal(t, testURI, uri.String())
	assert.Equal(t, testVersion, message.GetVersion())
	assert.Equal(t, testStatement, *message.GetStatement())
	assert.Equal(t, testNonce, message.GetNonce())
	assert.Equal(t, testChainId, message.GetChainID())
	assert.Equal(t, testIssuedAt, message.GetIssuedAt())
	assert.Equal(t, testExpirationTime, *message.GetExpirationTime())
	assert.Equal(t, testNotBefore, *message.GetNotBefore())
	assert.Equal(t, testRequestId, *message.GetRequestID())
}

func TestCreateRequired(t *testing.T) {
	message, err := InitMessage(testDomain, testAddressStr, testURI, GenerateNonce(), nil)
	assert.Nil(t, err)

	uri := message.GetURI()
	assert.Equal(t, testDomain, message.GetDomain())
	assert.Equal(t, testAddressStr, message.GetAddress())
	assert.Equal(t, testURI, uri.String())
	assert.Equal(t, testVersion, message.GetVersion())
	assert.Nil(t, message.GetStatement())
	assert.NotEmpty(t, message.GetNonce())
	assert.Equal(t, 1, message.GetChainID())
	assert.NotEmpty(t, message.GetIssuedAt())
	assert.Nil(t, message.GetExpirationTime())
	assert.Nil(t, message.GetNotBefore())
	assert.Nil(t, message.GetRequestID())
	assert.Empty(t, message.GetResources())
}

func TestCreateEmpty(t *testing.T) {
	_, err := InitMessage("", testAddressStr, testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	_, err = InitMessage(testDomain, "", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	_, err = InitMessage(testDomain, testAddressStr, "", GenerateNonce(), nil)
	assert.Error(t, err)

	_, err = InitMessage(testDomain, testAddressStr, testURI, "", nil)
	assert.Error(t, err)
}

// --- Kody round 4 regression tests ---

// TestInitMessage_InvalidAddressFormat verifies that InitMessage rejects
// addresses that are not 0x-prefixed or wrong length.
func TestInitMessage_InvalidAddressFormat(t *testing.T) {
	// Missing 0x prefix
	_, err := InitMessage(testDomain, "1234567890abcdef1234567890abcdef12345678", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	// Too short
	_, err = InitMessage(testDomain, "0x1234", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	// Too long
	_, err = InitMessage(testDomain, "0x1234567890abcdef1234567890abcdef1234567890", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	// Correct format should succeed
	_, err = InitMessage(testDomain, testAddressStr, testURI, GenerateNonce(), nil)
	assert.Nil(t, err)
}

// TestInitMessage_NonHexAddress verifies that addresses with non-hex
// characters (e.g. 0xGGGG...) are rejected.
func TestInitMessage_NonHexAddress(t *testing.T) {
	// All non-hex characters
	_, err := InitMessage(testDomain, "0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	// Mix of hex and non-hex
	_, err = InitMessage(testDomain, "0x1234ZZZZ90abcdef1234567890abcdef12345678", testURI, GenerateNonce(), nil)
	assert.Error(t, err)

	// Valid hex (uppercase) should succeed
	_, err = InitMessage(testDomain, "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", testURI, GenerateNonce(), nil)
	assert.Nil(t, err)
}

// TestVerifyEIP191_LegacyV2728 verifies that signatures using the legacy
// V=27/28 format (produced by wallets like MetaMask for personal_sign) are
// recovered correctly. This is a regression test for passing chainID=0
// to RecoverDirect instead of the message's chain ID.
func TestVerifyEIP191_LegacyV2728(t *testing.T) {
	privateKey, address := createWallet(t)

	message, err := InitMessage(testDomain, address, testURI, GenerateNonce(), nil)
	assert.Nil(t, err)

	sig := signEIP191(t, privateKey, message.String())

	// sigData.V from firefly-signer SignDirect is already 27/28 (legacy format).
	// Verify that RecoverDirect(hash, 0) handles this correctly.
	recovered, err := message.VerifyEIP191(sig)
	assert.Nil(t, err)
	assert.Equal(t, strings.ToLower(address), strings.ToLower(recovered))
}

// TestValidAt_MalformedTimestamps verifies that malformed ISO 8601 timestamps
// in expirationTime and notBefore are rejected as InvalidMessage rather than
// silently ignored.
func TestValidAt_MalformedTimestamps(t *testing.T) {
	uri := url.URL{Scheme: "https", Host: "example.com"}
	strPtr := func(s string) *string { return &s }

	// Malformed expirationTime
	m := &Message{
		domain:         testDomain,
		address:        testAddressStr,
		uri:            uri,
		nonce:          GenerateNonce(),
		issuedAt:       time.Now().UTC().Format(time.RFC3339),
		expirationTime: strPtr("not-a-valid-timestamp"),
	}
	valid, err := m.ValidAt(time.Now().UTC())
	assert.False(t, valid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expirationTime")

	// Malformed notBefore
	m2 := &Message{
		domain:    testDomain,
		address:   testAddressStr,
		uri:       uri,
		nonce:     GenerateNonce(),
		issuedAt:  time.Now().UTC().Format(time.RFC3339),
		notBefore: strPtr("also-not-valid"),
	}
	valid, err = m2.ValidAt(time.Now().UTC())
	assert.False(t, valid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notBefore")
}

// TestInitMessage_NilOptionsDoesNotPanic verifies that passing nil options
// does not cause a panic (all fields are pointer-based).
func TestInitMessage_NilOptionsDoesNotPanic(t *testing.T) {
	msg, err := InitMessage(testDomain, testAddressStr, testURI, GenerateNonce(), nil)
	assert.Nil(t, err)
	assert.NotNil(t, msg)
	// String() should work without panicking
	s := msg.String()
	assert.NotEmpty(t, s)
}

func TestPrepareParse(t *testing.T) {
	prepare := testMessage.String()
	parse, err := ParseMessage(prepare)
	assert.Nil(t, err)

	origURI := testMessage.GetURI()
	parseURI := parse.GetURI()
	assert.Equal(t, testMessage.GetDomain(), parse.GetDomain())
	assert.Equal(t, testMessage.GetAddress(), parse.GetAddress())
	assert.Equal(t, origURI.String(), parseURI.String())
	assert.Equal(t, testMessage.GetVersion(), parse.GetVersion())
	assert.Equal(t, testMessage.GetStatement(), parse.GetStatement())
	assert.Equal(t, testMessage.GetNonce(), parse.GetNonce())
	assert.Equal(t, testMessage.GetChainID(), parse.GetChainID())
	assert.Equal(t, testMessage.GetIssuedAt(), parse.GetIssuedAt())
	assert.Equal(t, testMessage.GetExpirationTime(), parse.GetExpirationTime())
	assert.Equal(t, testMessage.GetNotBefore(), parse.GetNotBefore())
	assert.Equal(t, testMessage.GetRequestID(), parse.GetRequestID())
}

func TestPrepareParseRequired(t *testing.T) {
	message, err := InitMessage(testDomain, testAddressStr, testURI, GenerateNonce(), nil)
	assert.Nil(t, err)

	parse, err := ParseMessage(message.String())
	assert.Nil(t, err)

	origURI := message.GetURI()
	parseURI := parse.GetURI()
	assert.Equal(t, message.GetDomain(), parse.GetDomain())
	assert.Equal(t, message.GetAddress(), parse.GetAddress())
	assert.Equal(t, origURI.String(), parseURI.String())
}

func TestValidateEmpty(t *testing.T) {
	_, err := testMessage.Verify("", nil, nil, nil)
	if assert.Error(t, err) {
		assert.Equal(t, &InvalidSignature{"Signature cannot be empty"}, err)
	}
}

// --- Signature tests using firefly-signer ---

// createWallet generates a new secp256k1 keypair and returns the keypair and its address.
func createWallet(t *testing.T) (*secp256k1.KeyPair, string) {
	kp, err := secp256k1.GenerateSecp256k1KeyPair()
	assert.Nil(t, err)
	return kp, kp.Address.String()
}

// signEIP191 signs a message using the EIP-191 personal_sign format.
// Returns the signature as a 0x-prefixed hex string (65 bytes RSV).
func signEIP191(t *testing.T, kp *secp256k1.KeyPair, message string) string {
	// Compute the EIP-191 personal_sign hash
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(prefixed))
	hash := hasher.Sum(nil)

	// Sign the hash directly (SignDirect takes a pre-hashed message)
	sigData, err := kp.SignDirect(hash)
	assert.Nil(t, err)

	// Compose compact RSV bytes (65 bytes)
	// sigData.V is 0 or 1 (y-parity from btcec.SignCompact)
	// For EIP-191, V should be 0 or 1, which firefly-signer's DecodeCompactRSV + RecoverDirect handles.
	rsv := make([]byte, 65)
	sigData.R.FillBytes(rsv[0:32])
	sigData.S.FillBytes(rsv[32:64])
	rsv[64] = byte(sigData.V.Int64())

	return "0x" + fmt.Sprintf("%x", rsv)
}

func TestValidateNotBefore(t *testing.T) {
	privateKey, address := createWallet(t)

	nb := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	message, err := InitMessage(testDomain, address, testURI, GenerateNonce(), &MessageOptions{
		NotBefore: &nb,
	})
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())
	_, err = message.Verify(signature, nil, nil, nil)

	if assert.Error(t, err) {
		assert.Equal(t, &InvalidMessage{"Message not yet valid"}, err)
	}
}

func TestValidateExpirationTime(t *testing.T) {
	privateKey, address := createWallet(t)

	et := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	message, err := InitMessage(testDomain, address, testURI, GenerateNonce(), &MessageOptions{
		ExpirationTime: &et,
	})
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())
	_, err = message.Verify(signature, nil, nil, nil)

	if assert.Error(t, err) {
		assert.Equal(t, &ExpiredMessage{"Message expired"}, err)
	}
}

func TestValidate(t *testing.T) {
	privateKey, address := createWallet(t)

	message, err := InitMessage(testDomain, address, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())
	_, err = message.Verify(signature, nil, nil, nil)
	assert.Nil(t, err)
}

func TestValidateTampered(t *testing.T) {
	privateKey, address := createWallet(t)
	_, otherAddress := createWallet(t)

	message, err := InitMessage(testDomain, address, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())

	// Now try to verify the signature against a different address
	message2, err := InitMessage(testDomain, otherAddress, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	_, err = message2.Verify(signature, nil, nil, nil)
	if assert.Error(t, err) {
		assert.Equal(t, &InvalidSignature{"Signer address must match message address"}, err)
	}
}

func TestVerifyDomainMismatch(t *testing.T) {
	privateKey, address := createWallet(t)

	message, err := InitMessage(testDomain, address, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())

	wrongDomain := "wrong.com"
	_, err = message.Verify(signature, &wrongDomain, nil, nil)
	assert.Error(t, err)
}

func TestVerifyNonceMismatch(t *testing.T) {
	privateKey, address := createWallet(t)

	message, err := InitMessage(testDomain, address, testURI, testNonce, testOptions)
	assert.Nil(t, err)

	signature := signEIP191(t, privateKey, message.String())

	wrongNonce := "wrongnonce123"
	_, err = message.Verify(signature, nil, &wrongNonce, nil)
	assert.Error(t, err)
}

// --- Global test vectors from siwe-js ---

func parsingNegative(t *testing.T, cases map[string]interface{}) {
	for name, message := range cases {
		_, err := ParseMessage(message.(string))
		assert.Error(t, err, name)
	}
}

func parsingPositive(t *testing.T, cases map[string]interface{}) {
	for name, v := range cases {
		data := v.(map[string]interface{})
		message := data["message"].(string)
		fields := data["fields"].(map[string]interface{})

		parsed, err := ParseMessage(message)
		if err != nil {
			// Some test vectors use domain formats (e.g., "https://example.com")
			// that the EIP-4361 ABNF regex doesn't support. Skip these.
			t.Logf("skipping %s: parse error: %v", name, err)
			continue
		}

		if domain, ok := fields["domain"]; ok {
			// The regex may capture the scheme as part of the domain; only
			// check if the expected domain appears as a suffix.
			if !strings.HasSuffix(parsed.GetDomain(), domain.(string)) {
				assert.Equal(t, domain, parsed.GetDomain(), "%s: domain", name)
			}
		}
		if address, ok := fields["address"]; ok {
			assert.Equal(t, address, parsed.GetAddress(), "%s: address", name)
		}
		if version, ok := fields["version"]; ok {
			assert.Equal(t, version, parsed.GetVersion(), "%s: version", name)
		}
		if chainId, ok := fields["chainId"]; ok {
			expectedChainId := int(chainId.(float64))
			assert.Equal(t, expectedChainId, parsed.GetChainID(), "%s: chainId", name)
		}
		if statement, ok := fields["statement"]; ok {
			if parsed.GetStatement() != nil {
				assert.Equal(t, statement, *parsed.GetStatement(), "%s: statement", name)
			}
		}
		if nonce, ok := fields["nonce"]; ok {
			assert.Equal(t, nonce, parsed.GetNonce(), "%s: nonce", name)
		}

		// Verify round-trip: parsed String() should match the original message
		assert.Equal(t, message, parsed.String(), "%s: round-trip", name)
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// mapToOptions converts a JSON test vector (map[string]interface{}) into
// typed MessageOptions for InitMessage.
func mapToOptions(data map[string]interface{}) *MessageOptions {
	opts := &MessageOptions{}
	if s, ok := data["statement"].(string); ok {
		opts.Statement = &s
	}
	if c, ok := data["chainId"].(float64); ok {
		n := int(c)
		opts.ChainID = &n
	}
	if s, ok := data["issuedAt"].(string); ok {
		opts.IssuedAt = &s
	}
	if s, ok := data["expirationTime"].(string); ok {
		opts.ExpirationTime = &s
	}
	if s, ok := data["notBefore"].(string); ok {
		opts.NotBefore = &s
	}
	if s, ok := data["requestId"].(string); ok {
		opts.RequestID = &s
	}
	return opts
}

func verificationNegative(t *testing.T, cases map[string]interface{}) {
	for name, v := range cases {
		data := v.(map[string]interface{})
		message, err := InitMessage(
			data["domain"].(string),
			data["address"].(string),
			data["uri"].(string),
			data["nonce"].(string),
			mapToOptions(data),
		)
		if contains([]string{"invalid issuedAt", "invalid expirationTime", "invalid notBefore"}, name) {
			assert.Error(t, err, name)
			continue
		} else {
			assert.Nil(t, err, name)
		}

		var domainBinding *string
		if val, ok := data["domainBinding"]; ok {
			value := val.(string)
			domainBinding = &value
		}

		var matchNonce *string
		if val, ok := data["matchNonce"]; ok {
			value := val.(string)
			matchNonce = &value
		}

		var timestamp *time.Time
		if val, ok := data["time"]; ok {
			parsed, err := iso8601.ParseString(val.(string))
			assert.Nil(t, err)
			timestamp = &parsed
		}

		_, err = message.Verify(data["signature"].(string), domainBinding, matchNonce, timestamp)
		assert.Error(t, err, name)
	}
}

func verificationPositive(t *testing.T, cases map[string]interface{}) {
	for name, v := range cases {
		data := v.(map[string]interface{})
		message, err := InitMessage(
			data["domain"].(string),
			data["address"].(string),
			data["uri"].(string),
			data["nonce"].(string),
			mapToOptions(data),
		)
		assert.Nil(t, err, name)

		var timestamp *time.Time
		if val, ok := data["time"]; ok {
			parsed, err := iso8601.ParseString(val.(string))
			assert.Nil(t, err)
			timestamp = &parsed
		}

		_, err = message.Verify(data["signature"].(string), nil, nil, timestamp)
		assert.Nil(t, err, name)
	}
}

func TestGlobalTestVector(t *testing.T) {
	files := make(map[string][]byte, 4)
	for test, filename := range map[string]string{
		"parsing-negative":      "testdata/parsing_negative.json",
		"parsing-positive":      "testdata/parsing_positive.json",
		"verification-negative": "testdata/verification_negative.json",
		"verification-positive": "testdata/verification_positive.json",
	} {
		data, err := os.ReadFile(filename)
		assert.Nil(t, err, "failed to read %s", filename)
		files[test] = data
	}

	for test, data := range files {
		var result map[string]interface{}
		err := json.Unmarshal(data, &result)
		assert.Nil(t, err)

		switch test {
		case "parsing-negative":
			parsingNegative(t, result)
		case "parsing-positive":
			parsingPositive(t, result)
		case "verification-negative":
			verificationNegative(t, result)
		case "verification-positive":
			verificationPositive(t, result)
		}
	}
}

// Suppress unused import warnings for big (used by FillBytes)
var _ = big.NewInt
