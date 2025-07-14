package core

import (
	"encoding/base64"
	"fmt"
	"github.com/ipfs/go-cid"
	b32 "github.com/multiformats/go-base32"
	"github.com/multiformats/go-multibase"
	mb "github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
	"slices"
	"unicode/utf8"
)

var (
	coreBase58Parser = &CoreBase58Parser{}
	coreHexParser    = &CoreHexParser{}
	coreBase64Parser = &CoreBase64Parser{}
	coreBase32Parser = &CoreBase32Parser{}
)

type StorageHash interface {
	Proof() []byte
	Multihash() mh.Multihash
	ProofExists() bool
	CIDType() uint64
	Type() uint64
	String() string
	Bytes() []byte
}

// StorageHashParser defines the interface for attempting to parse a string
// into a StorageHash.
type StorageHashParser interface {
	// TryParse attempts to parse the input string 's'.
	// It returns:
	// - StorageHash: The parsed hash if successful.
	// - bool: True if the parser recognized the format and attempted parsing
	//         (regardless of success). False if the format is definitively
	//         not handled by this parser.
	// - error: An error if parsing was attempted but failed validation or decoding.
	TryParse(s string) (StorageHash, bool, error)

	// ParserName provides a name for debugging/logging. Optional but helpful.
	ParserName() string
}

// ProtocolParser combines Protocol and StorageHashParser interfaces
type ProtocolParser interface {
	Protocol
	StorageHashParser
}

// GetParsersFromRegistry retrieves StorageHashParser implementations from registered
// protocols and appends the core fallback parsers.
// It returns an ordered list (protocols first, sorted by name, then fallbacks).
func GetParsersFromRegistry() []StorageHashParser {
	allProtocols := GetProtocolList() // Get sorted list of protocols
	fallbackParsers := []StorageHashParser{
		coreBase58Parser,
		coreBase32Parser,
		coreHexParser,
		coreBase64Parser,
	}
	parsers := make([]StorageHashParser, 0, len(allProtocols)+len(fallbackParsers)) // Pre-allocate approx size

	// 1. Add parsers from registered protocols
	for _, proto := range allProtocols {
		if parser, ok := proto.(StorageHashParser); ok {
			parsers = append(parsers, parser)
		}
	}

	// 2. Add core fallback parsers (append them last)
	// Note: The order here defines the fallback preference if a string could match multiple
	parsers = append(parsers, fallbackParsers...)

	return parsers
}

func NewStorageHashFromMultihashBytes(hash []byte, cidType uint64, proof []byte) StorageHash {
	multihash, err := mh.Cast(hash)

	if err != nil {
		return nil
	}

	decode, _ := mh.Decode(multihash)
	if decode == nil {
		return nil
	}

	return &StorageHashDefault{
		hash:    decode.Digest,
		typ:     decode.Code,
		proof:   proof,
		mh:      multihash,
		cidType: cidType,
	}
}

type StorageHashDefault struct {
	hash    []byte
	typ     uint64
	cidType uint64
	proof   []byte
	mh      mh.Multihash
}

func (s StorageHashDefault) Proof() []byte {
	return s.proof
}
func (s StorageHashDefault) ProofExists() bool {
	return len(s.proof) > 0
}

func (s StorageHashDefault) Multihash() mh.Multihash {
	if s.mh == nil {
		_mh, _ := mh.Encode(s.hash, s.typ)
		s.mh = _mh
	}

	return s.mh
}

func (s StorageHashDefault) CIDType() uint64 {
	return s.cidType
}

func (s StorageHashDefault) Type() uint64 {
	return s.typ
}

func (s StorageHashDefault) String() string {
	return s.Multihash().B58String()
}

func (s StorageHashDefault) Bytes() []byte {
	var c cid.Cid
	if s.cidType == 0 {
		c = cid.NewCidV0(s.Multihash())
	} else {
		c = cid.NewCidV1(s.cidType, s.Multihash())
	}
	return c.Bytes()
}

func NewStorageHash(hash []byte, typ uint64, cidType uint64, proof []byte) StorageHash {
	return &StorageHashDefault{
		hash:    hash,
		typ:     typ,
		cidType: cidType,
		proof:   proof,
	}
}

func NewStorageHashFromMultihash(hash mh.Multihash, cidType uint64, proof []byte) StorageHash {
	decode, _ := mh.Decode(hash)
	if decode == nil {
		return nil
	}

	return &StorageHashDefault{
		hash:    decode.Digest,
		typ:     decode.Code,
		proof:   proof,
		mh:      hash,
		cidType: cidType,
	}
}

func NewStorageHashFromRawMultihash(hash mh.Multihash) StorageHash {
	return NewStorageHashFromMultihash(hash, 0, nil)
}

func ParseStorageHash(s string) (StorageHash, error) {
	// Get parsers from the registry (includes core fallbacks)
	parsers := GetParsersFromRegistry()

	for _, parser := range parsers {
		sh, recognized, err := parser.TryParse(s)
		if recognized {
			// If the parser recognized the format (even if parsing failed),
			// we stop and return its result/error.
			return sh, err
		}
		// If not recognized, try the next parser.
	}

	// If no parser recognized the format
	return nil, ErrInvalidHashFormat
}

// CoreBase58Parser attempts to parse strings as standard Base58 encoded multihashes.
type CoreBase58Parser struct{}

// ParserName returns the identifier for this parser.
func (p *CoreBase58Parser) ParserName() string { return "CoreBase58Multihash" }

// TryParse implements the StorageHashParser interface for Base58 multihashes.
func (p *CoreBase58Parser) TryParse(s string) (StorageHash, bool, error) {
	// mh.FromB58String is quite strict. If it fails, it's unlikely the *intent*
	// was a base58 multihash unless it's simply malformed (e.g., invalid chars).
	// For a generic fallback, we are less forgiving than a protocol-specific parser.
	// If it doesn't decode cleanly, we assume it wasn't meant for this parser.
	hash, err := mh.FromB58String(s)
	if err != nil {
		// Any error from FromB58String means invalid Base58 format
		return nil, false, ErrInvalidHashFormat
	}

	// Successfully decoded Base58, now validate multihash structure
	decoded, err := mh.Decode(hash)
	if err != nil {
		// It decoded as Base58, but the resulting bytes aren't a valid multihash.
		// Return true for recognized, with an error.
		return nil, true, fmt.Errorf("decoded Base58 string resulted in invalid multihash structure: %w", err)
	}

	// Infer CID type based on multihash properties
	cidType := InferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))

	// Create the StorageHash object
	storageHash := NewStorageHashFromMultihash(hash, cidType, nil)

	return storageHash, true, nil // Success
}

// --- Fallback Hex Parser ---

// CoreHexParser attempts to parse strings as standard Hex encoded multihashes.
type CoreHexParser struct{}

// ParserName returns the identifier for this parser.
func (p *CoreHexParser) ParserName() string { return "CoreHexMultihash" }

// TryParse implements the StorageHashParser interface for Hex multihashes.
func (p *CoreHexParser) TryParse(s string) (StorageHash, bool, error) {
	// Avoid panics on empty or too-short strings
	if len(s) < 2 {
		return nil, false, nil
	}
	if !slices.Contains([]string{string(mb.Base32hex), string(mb.Base32hexUpper), string(mb.Base32hexPad), string(mb.Base32hexPadUpper)}, string(s[0])) {
		return nil, false, nil
	}

	// Decode the hex payload after the prefix
	hash, err := b32.HexEncoding.DecodeString(s[1:])
	if err != nil {
		return nil, false, nil // Not recognized as valid hex multihash format
	}

	// Successfully decoded Hex, now validate multihash structure
	decoded, err := mh.Decode(hash)
	if err != nil {
		// Decoded as Hex, but invalid multihash structure.
		return nil, true, fmt.Errorf("decoded Hex string resulted in invalid multihash structure: %w", err)
	}

	// Infer CID type
	cidType := InferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))

	// Create the StorageHash object
	storageHash := NewStorageHashFromMultihash(hash, cidType, nil)

	return storageHash, true, nil // Success
}

// --- Fallback Base64 Parser ---

// CoreBase64Parser handles parsing of Base64 encoded multihashes in both:
// - Standard RFC 4648 format (with/without padding)
// - Multibase-prefixed formats (Base64, Base64pad, Base64url, Base64urlPad)
//
// This parser supports both raw Base64 strings and those prefixed with multibase encoding indicators.
type CoreBase64Parser struct{}

// ParserName returns the identifier for this parser.
func (p *CoreBase64Parser) ParserName() string { return "CoreBase64Multihash" }

// TryParse implements the StorageHashParser interface for Base64 multihashes.
func (p *CoreBase64Parser) TryParse(s string) (StorageHash, bool, error) {
	// 1. Attempt standard Base64 decoding (RFC 4648)
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// If Base64 decoding fails, check if it might be multibase-prefixed
		return p.tryMultibaseBase64(s)
	}

	return p.parseDecodedBytes(decodedBytes, s)
}

func (p *CoreBase64Parser) tryMultibaseBase64(s string) (StorageHash, bool, error) {
	if len(s) == 0 {
		return nil, false, nil
	}

	r, _ := utf8.DecodeRuneInString(s)
	enc := multibase.Encoding(r)

	var decodedBytes []byte
	var err error

	switch enc {
	case multibase.Base64:
		decodedBytes, err = base64.StdEncoding.DecodeString(s[1:])
	case multibase.Base64pad:
		decodedBytes, err = base64.StdEncoding.DecodeString(s[1:])
	case multibase.Base64url:
		decodedBytes, err = base64.URLEncoding.DecodeString(s[1:])
	case multibase.Base64urlPad:
		decodedBytes, err = base64.URLEncoding.DecodeString(s[1:])
	default:
		// Not a recognized multibase Base64 format
		return nil, false, nil
	}

	if err != nil {
		return nil, true, fmt.Errorf("failed to decode multibase Base64 string: %w", err)
	}

	return p.parseDecodedBytes(decodedBytes, s)
}

func (p *CoreBase64Parser) parseDecodedBytes(decodedBytes []byte, original string) (StorageHash, bool, error) {
	if len(decodedBytes) < 2 {
		return nil, true, ErrInvalidHashFormat
	}

	// Try to interpret the decoded bytes as a multihash using Cast
	hash, err := mh.Cast(decodedBytes)
	if err != nil {
		return nil, true, fmt.Errorf("string is valid Base64 but content is not a valid multihash: %w", err)
	}

	// Successfully cast, now decode fully to get components for CID type inference.
	decoded, err := mh.Decode(hash)
	if err != nil {
		return nil, true, fmt.Errorf("internal error: failed to decode multihash after successful cast (from Base64): %w", err)
	}

	// Infer CID type
	cidType := InferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))

	// Create the StorageHash object
	storageHash := NewStorageHashFromMultihash(hash, cidType, nil)

	return storageHash, true, nil
}

// --- Fallback Base32 Parser ---

// CoreBase32Parser handles parsing of Base32 encoded multihashes in both:
// - Standard RFC 4648 format (with/without padding)
// - Multibase-prefixed formats (Base32, Base32Upper, Base32pad, Base32padUpper)
//
// This parser supports both raw Base32 strings and those prefixed with multibase encoding indicators.
// It's designed to be flexible while maintaining strict validation of the decoded multihash structure.
type CoreBase32Parser struct{}

// ParserName returns the identifier for this parser.
func (p *CoreBase32Parser) ParserName() string { return "CoreBase32Multihash" }

// TryParse implements the StorageHashParser interface for standard Base32 multihashes.
func (p *CoreBase32Parser) TryParse(s string) (StorageHash, bool, error) {
	if len(s) == 0 {
		return nil, false, nil
	}

	// First check if it has a multibase prefix
	r, _ := utf8.DecodeRuneInString(s)
	enc := multibase.Encoding(r)

	var decodedBytes []byte
	var decodeErr error

	switch enc {
	case multibase.Base32, multibase.Base32Upper, multibase.Base32pad, multibase.Base32padUpper:
		// Decode with multibase prefix
		decodedBytes, decodeErr = b32.StdEncoding.DecodeString(s[1:])
		if decodeErr != nil {
			return nil, true, fmt.Errorf("failed to decode multibase Base32 string: %w", decodeErr)
		}
	default:
		// Try standard Base32 decoding
		decodedBytes, decodeErr = b32.StdEncoding.DecodeString(s)
		if decodeErr != nil {
			return nil, false, nil
		}
	}

	if len(decodedBytes) < 2 {
		return nil, true, fmt.Errorf("decoded bytes too short to be a valid multihash")
	}

	// Check if we had a multibase prefix
	r, _ = utf8.DecodeRuneInString(s)
	enc = multibase.Encoding(r)
	hadMultibasePrefix := enc == multibase.Base32 || enc == multibase.Base32Upper ||
		enc == multibase.Base32pad || enc == multibase.Base32padUpper

	// If no multibase prefix and first rune is valid but not a base32 char,
	// don't recognize as base32 format
	if !hadMultibasePrefix && len(s) > 0 && r != utf8.RuneError &&
		!isBase32Char(r) {
		return nil, false, nil
	}

	// 2. Try to interpret the decoded bytes as a multihash using Cast
	if len(decodedBytes) < 2 {
		return nil, true, fmt.Errorf("decoded bytes too short to be a valid multihash")
	}

	hash, err := mh.Cast(decodedBytes)
	if err != nil {
		// It was valid Base32, but the content wasn't a valid multihash structure.
		return nil, true, fmt.Errorf("string is valid Base32 but content is not a valid multihash: %w", err)
	}

	// 3. Successfully cast, now decode fully to get components for CID type inference.
	decoded, err := mh.Decode(hash)
	if err != nil {
		// Should be unlikely if Cast succeeded, but handle defensively.
		return nil, true, fmt.Errorf("internal error: failed to decode multihash after successful cast (from Base32): %w", err)
	}

	// Infer CID type
	cidType := InferCIDTypeFromHashCode(decoded.Code, len(decoded.Digest))

	// Create the StorageHash object
	storageHash := NewStorageHashFromMultihash(hash, cidType, nil)

	return storageHash, true, nil // Success
}

func isBase32Char(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7') || r == '='
}

// InferCIDTypeFromHashCode maps hash algorithm codes to appropriate CID types
// Considers both hash type and length in determining the appropriate CID type
func InferCIDTypeFromHashCode(code uint64, digestLength int) uint64 {
	// Special case for SHA2-256 with 32-byte digest (IPFS compatibility)
	if code == mh.SHA2_256 && digestLength == 32 {
		// Check if this is likely an IPFS CIDv0 hash
		return 0x00 // CIDv0 for legacy IPFS content
	}

	// For identity hashes (direct content)
	if code == mh.IDENTITY {
		return 0x55 // Raw CID for identity hashes
	}

	// For raw binary data
	// This condition seems overly broad and might incorrectly classify things.
	// A more precise check might be needed depending on the exact definition of "Raw CID for small binary blobs".
	// For now, keeping the original logic but noting it might need refinement.
	if code == mh.SHA2_256 && digestLength <= 16 {
		return 0x55 // Raw CID for small binary blobs
	}

	// Default to CIDv1 for most other hash types
	return 0x01
}
