package core

import (
	"encoding/base64"
	"fmt"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
	"unicode/utf8"
)

// StorageHash represents a content hash with associated metadata.
// It provides a unified interface for working with different hash types.
// The underlying implementation may use CID or raw Multihash representations.
// Implementations should ensure immutability.  Methods that return byte slices
// should return copies to prevent modification of internal state.
type StorageHash interface {
	Proof() []byte
	Multihash() mh.Multihash
	ProofExists() bool
	CIDType() uint64
	Type() uint64
	String() string
	CIDString() string
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
	allProtocols := GetProtocolList()       // Get sorted list of protocols
	fallbackParsers := []StorageHashParser{ // Order matters!
		&CIDStorageHashParser{},
		&MultihashStorageHashParser{},
		&CoreBase64Parser{}, // Keep base64 parser as a last resort
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

// CID returns the CID representation of this storage hash.
func (s StorageHashDefault) CID() cid.Cid {
	if s.cidType == 0 {
		return cid.NewCidV0(s.Multihash())
	}
	return cid.NewCidV1(s.cidType, s.Multihash())
}

// CIDString returns the CIDv1 (or CIDv0 for legacy) string representation
// of this storage hash. This is the canonical, human-readable identifier
// for content-addressed data, suitable for use in trace attributes, logs,
// and cross-referencing with IPFS gateways.
func (s StorageHashDefault) CIDString() string {
	return s.CID().String()
}

func (s StorageHashDefault) Bytes() []byte {
	return s.CID().Bytes()
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

// CIDStorageHashParser wraps the go-cid library for StorageHash parsing.
type CIDStorageHashParser struct{}

// ParserName returns the identifier for this parser.
func (p *CIDStorageHashParser) ParserName() string {
	return "CIDStorageHashParser"
}

// TryParse implements the StorageHashParser interface using cid.Decode.
func (p *CIDStorageHashParser) TryParse(s string) (StorageHash, bool, error) {
	c, err := cid.Decode(s)
	if err != nil {
		// Not a valid CID
		return nil, false, nil
	}

	// Successfully parsed as a CID
	return NewStorageHashFromMultihash(c.Hash(), uint64(c.Type()), nil), true, nil
}

// MultihashStorageHashParser attempts to parse strings as raw multihashes (base58 encoded).
type MultihashStorageHashParser struct{}

// ParserName returns the identifier for this parser.
func (p *MultihashStorageHashParser) ParserName() string {
	return "MultihashStorageHashParser"
}

// TryParse implements the StorageHashParser interface for raw multihashes.
func (p *MultihashStorageHashParser) TryParse(s string) (StorageHash, bool, error) {
	hash, err := mh.FromB58String(s)
	if err != nil {
		return nil, false, nil // Not recognized as a valid base58 multihash
	}

	// Successfully decoded as base58 multihash
	return NewStorageHashFromRawMultihash(hash), true, nil
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
