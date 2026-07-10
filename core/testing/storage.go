package testing

import (
	"io"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
)

var _ core.StorageHash = (*MockStorageHash)(nil)

// MockStorageHash implements core.StorageHash for testing
type MockStorageHash struct {
	ProofValue       []byte
	MultihashValue   mh.Multihash
	ProofExistsValue bool
	CIDTypeValue     uint64
	TypeValue        uint64
}

func (s *MockStorageHash) Proof() []byte {
	return s.ProofValue
}

func (s *MockStorageHash) Multihash() mh.Multihash {
	return s.MultihashValue
}

func (s *MockStorageHash) ProofExists() bool {
	return s.ProofExistsValue
}

func (s *MockStorageHash) CIDType() uint64 {
	return s.CIDTypeValue
}

func (s *MockStorageHash) Type() uint64 {
	return s.TypeValue
}

func (s *MockStorageHash) String() string {
	return s.MultihashValue.String()
}

func (s *MockStorageHash) CIDString() string {
	if s.MultihashValue == nil {
		return ""
	}
	if s.CIDTypeValue == 0 {
		return cid.NewCidV0(s.MultihashValue).String()
	}
	return cid.NewCidV1(s.CIDTypeValue, s.MultihashValue).String()
}

// Bytes returns the binary representation of the storage hash as a CID (Content Identifier).
// It handles both CIDv0 (for legacy IPFS hashes) and CIDv1 formats based on CIDTypeValue.
// Returns nil if MultihashValue is nil to prevent panics during testing.
func (s *MockStorageHash) Bytes() []byte {
	if s.MultihashValue == nil {
		return nil
	}

	if s.CIDTypeValue == 0 {
		cid := cid.NewCidV0(s.MultihashValue)
		return cid.Bytes()
	}
	cid := cid.NewCidV1(s.CIDTypeValue, s.MultihashValue)
	return cid.Bytes()
}

// NewMockStorageHash creates a new mock storage hash
func NewMockStorageHash() *MockStorageHash {
	return &MockStorageHash{
		ProofExistsValue: false,
	}
}

// WithProof sets the proof for the mock storage hash
func (s *MockStorageHash) WithProof(proof []byte) *MockStorageHash {
	s.ProofValue = proof
	s.ProofExistsValue = len(proof) > 0
	return s
}

// WithMultihash sets the multihash for the mock storage hash
func (s *MockStorageHash) WithMultihash(hash mh.Multihash) *MockStorageHash {
	s.MultihashValue = hash
	return s
}

// WithCIDType sets the CID type for the mock storage hash
func (s *MockStorageHash) WithCIDType(cidType uint64) *MockStorageHash {
	s.CIDTypeValue = cidType
	return s
}

// WithType sets the type for the mock storage hash
func (s *MockStorageHash) WithType(typ uint64) *MockStorageHash {
	s.TypeValue = typ
	return s
}

// MockStorageProtocol implements core.StorageProtocol for testing
type MockStorageProtocol struct {
	NameValue          string
	EncodeFileNameFunc func(core.StorageHash) string
	HashFunc           func(r io.Reader, size uint64) (core.StorageHash, error)
}

func (p *MockStorageProtocol) Name() string {
	return p.NameValue
}

func (p *MockStorageProtocol) EncodeFileName(hash core.StorageHash) string {
	if p.EncodeFileNameFunc != nil {
		return p.EncodeFileNameFunc(hash)
	}
	return "mock-filename"
}

func (p *MockStorageProtocol) Hash(r io.Reader, size uint64) (core.StorageHash, error) {
	if p.HashFunc != nil {
		return p.HashFunc(r, size)
	}
	return NewMockStorageHash(), nil
}

// NewMockStorageProtocol creates a new mock storage protocol
func NewMockStorageProtocol(name string) *MockStorageProtocol {
	return &MockStorageProtocol{
		NameValue: name,
	}
}

// MockStorageUploadRequest implements core.StorageUploadRequest for testing
type MockStorageUploadRequest struct {
	ProtocolValue  core.StorageProtocol
	DataValue      io.ReadSeeker
	SizeValue      uint64
	MuParamsValue  *core.MultipartUploadParams
	HashValue      core.StorageHash
	HashTypesValue []uint64
	HashesValue    []core.StorageHash
}

func (r *MockStorageUploadRequest) Protocol() core.StorageProtocol {
	return r.ProtocolValue
}

func (r *MockStorageUploadRequest) SetProtocol(protocol core.StorageProtocol) {
	r.ProtocolValue = protocol
}

func (r *MockStorageUploadRequest) Data() io.ReadSeeker {
	return r.DataValue
}

func (r *MockStorageUploadRequest) SetData(data io.ReadSeeker) {
	r.DataValue = data
}

func (r *MockStorageUploadRequest) Size() uint64 {
	return r.SizeValue
}

func (r *MockStorageUploadRequest) SetSize(size uint64) {
	r.SizeValue = size
}

func (r *MockStorageUploadRequest) MuParams() *core.MultipartUploadParams {
	return r.MuParamsValue
}

func (r *MockStorageUploadRequest) SetMuParams(params *core.MultipartUploadParams) {
	r.MuParamsValue = params
}

func (r *MockStorageUploadRequest) Hash() core.StorageHash {
	return r.HashValue
}

func (r *MockStorageUploadRequest) SetHash(hash core.StorageHash) {
	r.HashValue = hash
}

func (r *MockStorageUploadRequest) HashTypes() []uint64 {
	return r.HashTypesValue
}

func (r *MockStorageUploadRequest) SetHashTypes(types []uint64) {
	r.HashTypesValue = types
}

func (r *MockStorageUploadRequest) Hashes() []core.StorageHash {
	return r.HashesValue
}

func (r *MockStorageUploadRequest) SetHashes(hashes []core.StorageHash) {
	r.HashesValue = hashes
}

// NewMockStorageUploadRequest creates a new mock storage upload request
func NewMockStorageUploadRequest() *MockStorageUploadRequest {
	return &MockStorageUploadRequest{}
}
