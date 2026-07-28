// Vendored and adapted from github.com/spruceid/siwe-go.
// Copyright (c) 2021 Spruce Systems, Inc.
// Licensed under Apache License 2.0 / MIT (dual-licensed).
//
// See THIRD_PARTY_NOTICES.md for full attribution and a summary of
// modifications made to the original source code.
//
// The upstream library (github.com/spruceid/siwe-go) is unmaintained, so we
// vendor the EIP-4361 parsing, message construction, and verification logic
// here. go-ethereum crypto dependencies have been replaced with
// hyperledger-firefly/signer for signature recovery.

package caip122

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dchest/uniuri"
	"github.com/hyperledger-firefly/signer/pkg/secp256k1"
	"github.com/relvacode/iso8601"
	"golang.org/x/crypto/sha3"
)

// --- Error types (ported from siwe-go/errors.go) ---

type ExpiredMessage struct{ Msg string }
type InvalidMessage struct{ Msg string }
type InvalidSignature struct{ Msg string }

func (m *ExpiredMessage) Error() string {
	return fmt.Sprintf("Expired Message: %s", m.Msg)
}

func (m *InvalidMessage) Error() string {
	return fmt.Sprintf("Invalid Message: %s", m.Msg)
}

func (m *InvalidSignature) Error() string {
	return fmt.Sprintf("Invalid Signature: %s", m.Msg)
}

// --- Message struct (ported from siwe-go/message.go) ---

// Message represents a parsed EIP-4361 (CAIP-122 ethereum namespace) sign-in message.
type Message struct {
	domain         string
	address        string
	uri            url.URL
	version        string
	statement      *string
	nonce          string
	chainID        int
	issuedAt       string
	expirationTime *string
	notBefore      *string
	requestID      *string
	resources      []url.URL
}

func (m *Message) GetDomain() string {
	return m.domain
}

func (m *Message) GetAddress() string {
	return m.address
}

func (m *Message) GetURI() url.URL {
	return m.uri
}

func (m *Message) GetVersion() string {
	return m.version
}

func (m *Message) GetStatement() *string {
	if m.statement != nil {
		ret := *m.statement
		return &ret
	}
	return nil
}

func (m *Message) GetNonce() string {
	return m.nonce
}

func (m *Message) GetChainID() int {
	return m.chainID
}

// GetChainIDString returns the chain_id as a CAIP-2 identifier string (e.g., "eip155:1").
func (m *Message) GetChainIDString() string {
	return fmt.Sprintf("eip155:%d", m.chainID)
}

func (m *Message) GetIssuedAt() string {
	return m.issuedAt
}

func (m *Message) GetExpirationTime() *string {
	if m.expirationTime != nil {
		ret := *m.expirationTime
		return &ret
	}
	return nil
}

func (m *Message) GetNotBefore() *string {
	if m.notBefore != nil {
		ret := *m.notBefore
		return &ret
	}
	return nil
}

func (m *Message) GetRequestID() *string {
	if m.requestID != nil {
		ret := *m.requestID
		return &ret
	}
	return nil
}

func (m *Message) GetResources() []url.URL {
	return m.resources
}

// --- Regex parser (ported from siwe-go/regex.go) ---

const _SIWE_DOMAIN = `(?P<domain>([^/?#]+)) wants you to sign in with your Ethereum account:\n`
const _SIWE_ADDRESS = `(?P<address>0x[a-zA-Z0-9]{40})\n\n`
const _SIWE_STATEMENT = `((?P<statement>[^\n]+)\n)?\n`
const _RFC3986 = `(([^ :/?#]+):)?(//([^ /?#]*))?([^ ?#]*)(\?([^ #]*))?(#(.*))?`

var _SIWE_URI_LINE = fmt.Sprintf("URI: (?P<uri>%s?)\n", _RFC3986)

const _SIWE_VERSION = `Version: (?P<version>1)\n`
const _SIWE_CHAIN_ID = `Chain ID: (?P<chainId>[0-9]+)\n`
const _SIWE_NONCE = `Nonce: (?P<nonce>[a-zA-Z0-9]{8,})\n`
const _SIWE_DATETIME = `([0-9]+)-(0[1-9]|1[012])-(0[1-9]|[12][0-9]|3[01])[Tt]([01][0-9]|2[0-3]):([0-5][0-9]):([0-5][0-9]|60)(\.[0-9]+)?(([Zz])|([\\+|\\-]([01][0-9]|2[0-3]):[0-5][0-9]))`

var _SIWE_ISSUED_AT = fmt.Sprintf("Issued At: (?P<issuedAt>%s)", _SIWE_DATETIME)
var _SIWE_EXPIRATION_TIME = fmt.Sprintf(`(\nExpiration Time: (?P<expirationTime>%s))?`, _SIWE_DATETIME)
var _SIWE_NOT_BEFORE = fmt.Sprintf(`(\nNot Before: (?P<notBefore>%s))?`, _SIWE_DATETIME)

const _SIWE_REQUEST_ID = `(\nRequest ID: (?P<requestId>[-._~!$&'()*+,;=:@%a-zA-Z0-9]*))?`

var _SIWE_RESOURCES = fmt.Sprintf(`(\nResources:(?P<resources>(\n- %s)+))?`, _RFC3986)

var _SIWE_MESSAGE = regexp.MustCompile(fmt.Sprintf("^%s%s%s%s%s%s%s%s%s%s%s%s$",
	_SIWE_DOMAIN,
	_SIWE_ADDRESS,
	_SIWE_STATEMENT,
	_SIWE_URI_LINE,
	_SIWE_VERSION,
	_SIWE_CHAIN_ID,
	_SIWE_NONCE,
	_SIWE_ISSUED_AT,
	_SIWE_EXPIRATION_TIME,
	_SIWE_NOT_BEFORE,
	_SIWE_REQUEST_ID,
	_SIWE_RESOURCES))

// --- Utils (ported from siwe-go/utils.go) ---

func isEmpty(str *string) bool {
	return str == nil || *str == ""
}

func GenerateNonce() string {
	return uniuri.NewLen(16)
}

// --- SIWE logic (ported from siwe-go/siwe.go) ---

func buildAuthority(uri *url.URL) string {
	authority := uri.Host
	if uri.User != nil {
		authority = fmt.Sprintf("%s@%s", uri.User.String(), authority)
	}
	return authority
}

func validateDomain(domain *string) (bool, error) {
	if isEmpty(domain) {
		return false, &InvalidMessage{"`domain` must not be empty"}
	}

	validateDomain, err := url.Parse(fmt.Sprintf("https://%s", *domain))
	if err != nil {
		return false, &InvalidMessage{"Invalid format for field `domain`"}
	}

	authority := buildAuthority(validateDomain)
	if authority != *domain {
		return false, &InvalidMessage{"Invalid format for field `domain`"}
	}

	return true, nil
}

func validateURI(uri *string) (*url.URL, error) {
	if isEmpty(uri) {
		return nil, &InvalidMessage{"`uri` must not be empty"}
	}

	validateURI, err := url.Parse(*uri)
	if err != nil {
		return nil, &InvalidMessage{"Invalid format for field `uri`"}
	}

	return validateURI, nil
}

// MessageOptions provides typed optional parameters for InitMessage.
// This replaces the map[string]interface{} pattern inherited from siwe-go,
// eliminating all unchecked type assertions.
type MessageOptions struct {
	Statement      *string
	ChainID        *int
	IssuedAt       *string   // ISO 8601 formatted
	ExpirationTime *string   // ISO 8601 formatted
	NotBefore      *string   // ISO 8601 formatted
	RequestID      *string
	Resources      []url.URL
}

// InitMessage creates a Message object with the provided parameters.
func InitMessage(domain, address, uri, nonce string, opts *MessageOptions) (*Message, error) {
	if opts == nil {
		opts = &MessageOptions{}
	}

	if ok, err := validateDomain(&domain); !ok {
		return nil, err
	}

	if isEmpty(&address) {
		return nil, &InvalidMessage{"`address` must not be empty"}
	}
	if !strings.HasPrefix(address, "0x") && !strings.HasPrefix(address, "0X") {
		return nil, &InvalidMessage{"`address` must be 0x-prefixed"}
	}
	if len(address) != 42 {
		return nil, &InvalidMessage{"`address` must be 42 characters"}
	}
	if _, err := hex.DecodeString(address[2:]); err != nil {
		return nil, &InvalidMessage{"`address` must be a valid hex string"}
	}

	validateURIResult, err := validateURI(&uri)
	if err != nil {
		return nil, err
	}

	if isEmpty(&nonce) {
		return nil, &InvalidMessage{"`nonce` must not be empty"}
	}

	chainId := 1
	if opts.ChainID != nil {
		chainId = *opts.ChainID
	}

	var issuedAt string
	if opts.IssuedAt != nil {
		_, err := iso8601.ParseString(*opts.IssuedAt)
		if err != nil {
			return nil, &InvalidMessage{"Invalid format for field `issuedAt`"}
		}
		issuedAt = *opts.IssuedAt
	} else {
		issuedAt = time.Now().UTC().Format(time.RFC3339)
	}

	var expirationTime *string
	if opts.ExpirationTime != nil {
		_, err := iso8601.ParseString(*opts.ExpirationTime)
		if err != nil {
			return nil, &InvalidMessage{"Invalid format for field `expirationTime`"}
		}
		expirationTime = opts.ExpirationTime
	}

	var notBefore *string
	if opts.NotBefore != nil {
		_, err := iso8601.ParseString(*opts.NotBefore)
		if err != nil {
			return nil, &InvalidMessage{"Invalid format for field `notBefore`"}
		}
		notBefore = opts.NotBefore
	}

	// Normalize the address to EIP-55 checksum format for consistent output.
	// Note: we don't enforce EIP-55 on construction (only on parsing), matching siwe-go behavior.
	normalizedAddress := normalizeAddress(address)

	return &Message{
		domain:         domain,
		address:        normalizedAddress,
		uri:            *validateURIResult,
		version:        "1",
		statement:      opts.Statement,
		nonce:          nonce,
		chainID:        chainId,
		issuedAt:       issuedAt,
		expirationTime: expirationTime,
		notBefore:      notBefore,
		requestID:      opts.RequestID,
		resources:      opts.Resources,
	}, nil
}

func parseMessage(message string) (map[string]interface{}, error) {
	match := _SIWE_MESSAGE.FindStringSubmatch(message)

	if match == nil {
		return nil, &InvalidMessage{"Message could not be parsed"}
	}

	result := make(map[string]interface{})
	subexpNames := _SIWE_MESSAGE.SubexpNames()
	for i, name := range subexpNames {
		if i != 0 && name != "" && match[i] != "" {
			result[name] = match[i]
		}
	}

	if _, ok := result["domain"]; !ok {
		return nil, &InvalidMessage{"`domain` must not be empty"}
	}
	domain := result["domain"].(string)
	if ok, err := validateDomain(&domain); !ok {
		return nil, err
	}

	if _, ok := result["uri"]; !ok {
		return nil, &InvalidMessage{"`uri` must not be empty"}
	}
	uri := result["uri"].(string)
	if _, err := validateURI(&uri); err != nil {
		return nil, err
	}

	originalAddress := result["address"].(string)
	parsedAddress := normalizeAddress(originalAddress)
	if originalAddress != parsedAddress {
		return nil, &InvalidMessage{"Address must be in EIP-55 format"}
	}

	if val, ok := result["resources"]; ok {
		resources := strings.Split(val.(string), "\n- ")[1:]
		validateResources := make([]url.URL, len(resources))
		for i, resource := range resources {
			validateResource, err := url.Parse(resource)
			if err != nil {
				return nil, &InvalidMessage{fmt.Sprintf("Invalid format for field `resources` at position %d", i)}
			}
			validateResources[i] = *validateResource
		}
		result["resources"] = validateResources
	}

	return result, nil
}

// ParseMessage returns a Message object by parsing an EIP-4361 formatted string.
func ParseMessage(message string) (*Message, error) {
	result, err := parseMessage(message)
	if err != nil {
		return nil, err
	}

	// Build typed options from the parsed map.
	opts := &MessageOptions{}

	if s, ok := result["statement"].(string); ok {
		opts.Statement = &s
	}
	if s, ok := result["chainId"].(string); ok {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, &InvalidMessage{"Invalid format for field `chainId`"}
		}
		opts.ChainID = &n
	}
	if s, ok := result["issuedAt"].(string); ok {
		opts.IssuedAt = &s
	}
	if s, ok := result["expirationTime"].(string); ok {
		opts.ExpirationTime = &s
	}
	if s, ok := result["notBefore"].(string); ok {
		opts.NotBefore = &s
	}
	if s, ok := result["requestId"].(string); ok {
		opts.RequestID = &s
	}
	if resources, ok := result["resources"].([]url.URL); ok {
		opts.Resources = resources
	}

	parsed, err := InitMessage(
		result["domain"].(string),
		result["address"].(string),
		result["uri"].(string),
		result["nonce"].(string),
		opts,
	)

	if err != nil {
		return nil, err
	}

	return parsed, nil
}

func (m *Message) eip191Hash() []byte {
	data := []byte(m.String())
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(msg))
	return hasher.Sum(nil)
}

// ValidNow validates the time constraints of the message at current time.
func (m *Message) ValidNow() (bool, error) {
	return m.ValidAt(time.Now().UTC())
}

// ValidAt validates the time constraints of the message at a specific point in time.
func (m *Message) ValidAt(when time.Time) (bool, error) {
	if m.expirationTime != nil {
		expTime, err := iso8601.ParseString(*m.expirationTime)
		if err != nil {
			return false, &InvalidMessage{"Invalid format for field `expirationTime`"}
		}
		if when.After(expTime) {
			return false, &ExpiredMessage{"Message expired"}
		}
	}

	if m.notBefore != nil {
		nbTime, err := iso8601.ParseString(*m.notBefore)
		if err != nil {
			return false, &InvalidMessage{"Invalid format for field `notBefore`"}
		}
		if when.Before(nbTime) {
			return false, &InvalidMessage{"Message not yet valid"}
		}
	}

	return true, nil
}

// VerifyEIP191 validates the integrity of the object by matching its signature.
// Uses firefly-signer for secp256k1 recovery (no go-ethereum dependency).
// Returns the recovered Ethereum address (lowercase, 0x-prefixed) if valid.
func (m *Message) VerifyEIP191(signature string) (string, error) {
	if isEmpty(&signature) {
		return "", &InvalidSignature{"Signature cannot be empty"}
	}

	sigBytes, err := decodeHex(signature)
	if err != nil {
		return "", &InvalidSignature{"Failed to decode signature"}
	}

	if len(sigBytes) != 65 {
		return "", &InvalidSignature{fmt.Sprintf("Invalid signature length: expected 65 bytes, got %d", len(sigBytes))}
	}

	// Use firefly-signer to decode and recover.
	sigData, err := secp256k1.DecodeCompactRSV(context.Background(), sigBytes)
	if err != nil {
		return "", &InvalidSignature{"Failed to decode signature"}
	}

	// For EIP-191, V should be 0/1 (raw y-parity) or 27/28 (legacy).
	// RecoverDirect with chainID=0 handles all V value formats correctly:
	// 0/1 → used directly, 27/28 → mapped to 0/1.
	recoveredAddr, err := sigData.RecoverDirect(m.eip191Hash(), 0)
	if err != nil {
		return "", &InvalidSignature{"Failed to recover public key from signature"}
	}

	recovered := strings.ToLower(recoveredAddr.String())
	expected := strings.ToLower(m.address)
	if recovered != expected {
		return "", &InvalidSignature{"Signer address must match message address"}
	}

	return recovered, nil
}

// Verify validates time constraints and integrity of the object by matching its signature.
// Optional domain and nonce parameters, if non-nil, must match the message's values.
// Optional timestamp, if non-nil, is used instead of time.Now() for time constraint checks.
// Returns the recovered Ethereum address (lowercase, 0x-prefixed) if valid.
func (m *Message) Verify(signature string, domain *string, nonce *string, timestamp *time.Time) (string, error) {
	var err error

	if timestamp != nil {
		_, err = m.ValidAt(*timestamp)
	} else {
		_, err = m.ValidNow()
	}

	if err != nil {
		return "", err
	}

	if domain != nil {
		if m.GetDomain() != *domain {
			return "", &InvalidSignature{"Message domain doesn't match"}
		}
	}

	if nonce != nil {
		if m.GetNonce() != *nonce {
			return "", &InvalidSignature{"Message nonce doesn't match"}
		}
	}

	return m.VerifyEIP191(signature)
}

func (m *Message) prepareMessage() string {
	greeting := fmt.Sprintf("%s wants you to sign in with your Ethereum account:", m.domain)
	headerArr := []string{greeting, m.address}

	if isEmpty(m.statement) {
		headerArr = append(headerArr, "\n")
	} else {
		headerArr = append(headerArr, fmt.Sprintf("\n%s\n", *m.statement))
	}

	header := strings.Join(headerArr, "\n")

	uri := fmt.Sprintf("URI: %s", m.uri.String())
	version := fmt.Sprintf("Version: %s", m.version)
	chainId := fmt.Sprintf("Chain ID: %d", m.chainID)
	nonce := fmt.Sprintf("Nonce: %s", m.nonce)
	issuedAt := fmt.Sprintf("Issued At: %s", m.issuedAt)

	bodyArr := []string{uri, version, chainId, nonce, issuedAt}

	if !isEmpty(m.expirationTime) {
		value := fmt.Sprintf("Expiration Time: %s", *m.expirationTime)
		bodyArr = append(bodyArr, value)
	}

	if !isEmpty(m.notBefore) {
		value := fmt.Sprintf("Not Before: %s", *m.notBefore)
		bodyArr = append(bodyArr, value)
	}

	if !isEmpty(m.requestID) {
		value := fmt.Sprintf("Request ID: %s", *m.requestID)
		bodyArr = append(bodyArr, value)
	}

	if len(m.resources) > 0 {
		resourcesArr := make([]string, len(m.resources))
		for i, v := range m.resources {
			resourcesArr[i] = fmt.Sprintf("- %s", v.String())
		}

		resources := strings.Join(resourcesArr, "\n")
		value := fmt.Sprintf("Resources:\n%s", resources)

		bodyArr = append(bodyArr, value)
	}

	body := strings.Join(bodyArr, "\n")

	return strings.Join([]string{header, body}, "\n")
}

func (m *Message) String() string {
	return m.prepareMessage()
}

// FormatMessage constructs an EIP-4361 (CAIP-122) sign-in message for the
// given address, domain, nonce, and chain ID. The TTL determines the
// Expiration Time, matching the stored nonce's lifetime.
// chainID must be a CAIP-2 identifier string (e.g., "eip155:1").
func FormatMessage(address, domain, nonce, chainID string, ttl time.Duration) (string, error) {
	// Parse chainID as CAIP-2 (eip155:<number>)
	chainNum, err := parseChainIDNumber(chainID)
	if err != nil {
		return "", fmt.Errorf("caip122: invalid chain_id: %w", err)
	}

	uri := "https://" + domain
	now := time.Now().UTC()
	issuedAt := now.Format(time.RFC3339)
	expiration := now.Add(ttl).Format(time.RFC3339)

	opts := &MessageOptions{
		ChainID:        &chainNum,
		IssuedAt:       &issuedAt,
		ExpirationTime: &expiration,
	}

	msg, err := InitMessage(domain, address, uri, nonce, opts)
	if err != nil {
		return "", fmt.Errorf("caip122: failed to construct message: %w", err)
	}

	return msg.String(), nil
}

// --- Helpers ---

// normalizeAddress returns the EIP-55 checksummed representation of a hex address.
// We implement this ourselves to avoid pulling in go-ethereum's common.HexToAddress.
func normalizeAddress(addr string) string {
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return addr
	}
	hexPart := addr[2:]
	if len(hexPart) != 40 {
		return addr
	}
	// Compute EIP-55 checksum
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(strings.ToLower(hexPart)))
	hash := hasher.Sum(nil)

	result := make([]byte, 42)
	result[0] = '0'
	result[1] = 'x'
	for i := 0; i < 40; i++ {
		c := hexPart[i]
		if c >= '0' && c <= '9' {
			result[i+2] = c
		} else {
			// Use hash to determine case
			hashNibble := hash[i/2]
			if i%2 == 0 {
				hashNibble >>= 4
			} else {
				hashNibble &= 0xf
			}
			if hashNibble >= 8 {
				result[i+2] = byteToUpper(c)
			} else {
				result[i+2] = byteToLower(c)
			}
		}
	}
	return string(result)
}

func byteToUpper(c byte) byte {
	if c >= 'a' && c <= 'f' {
		return c - 32
	}
	return c
}

func byteToLower(c byte) byte {
	if c >= 'A' && c <= 'F' {
		return c + 32
	}
	return c
}

// decodeHex decodes a hex string, optionally stripping a 0x prefix.
func decodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return hex.DecodeString(s)
}

// parseChainIDNumber extracts the integer chain ID from a CAIP-2 identifier
// (e.g., "eip155:1" → 1).
func parseChainIDNumber(caip2 string) (int, error) {
	parts := strings.SplitN(caip2, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid CAIP-2 format: %s", caip2)
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid chain ID number: %s", parts[1])
	}
	return n, nil
}

// verifySignature recovers the signer address from an EIP-191 personal_sign
// signature against a CAIP-122 message. It does NOT validate nonce, domain,
// or timestamp — authentication code must use ChallengeService.VerifyChallenge
// or Message.Verify so those fields are validated against the challenge store.
func verifySignature(message string, signature string) (string, error) {
	msg, err := ParseMessage(message)
	if err != nil {
		return "", fmt.Errorf("caip122: failed to parse message: %w", err)
	}
	return msg.VerifyEIP191(signature)
}
