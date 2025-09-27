package testing

import (
	"context"
	"io"

	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"gorm.io/gorm"
)

var _ core.StorageProtocol = (*MockProtocol)(nil)
var _ core.Protocol = (*MockProtocol)(nil)

// MockProtocol implements core.Protocol for testing
type MockProtocol struct {
	NameValue          string
	DisplayNameValue   string
	ConfigValue        config.ProtocolConfig
	OperationsValue    []core.Operation
	WorkflowsValue     []core.WorkflowDefinition
	EncodeFileNameFunc func(core.StorageHash) string
	HashFunc           func(r io.Reader, size uint64) (core.StorageHash, error)
	PinHandlerValue    core.ProtocolPinHandler
	tb                 TB // Store the test instance
}

// TB returns the stored test instance
func (p *MockProtocol) TB() TB {
	return p.tb
}

func (p *MockProtocol) Name() string {
	return p.NameValue
}

func (p *MockProtocol) DisplayName() string {
	if p.DisplayNameValue != "" {
		return p.DisplayNameValue
	}
	return p.NameValue
}

func (p *MockProtocol) Config() config.ProtocolConfig {
	return p.ConfigValue
}

func (p *MockProtocol) Operations() []core.Operation {
	return p.OperationsValue
}

func (p *MockProtocol) EncodeFileName(hash core.StorageHash) string {
	if p.EncodeFileNameFunc != nil {
		return p.EncodeFileNameFunc(hash)
	}
	return ""
}

func (p *MockProtocol) Hash(r io.Reader, size uint64) (core.StorageHash, error) {
	if p.HashFunc != nil {
		return p.HashFunc(r, size)
	}
	return nil, nil
}

func (p *MockProtocol) PinHandler() core.ProtocolPinHandler {
	return p.PinHandlerValue
}

func (p *MockProtocol) Workflows() []core.WorkflowDefinition {
	return p.WorkflowsValue
}

// NewMockProtocol creates a new mock protocol with default mock implementations
func NewMockProtocol(t TB, name string) *MockProtocol {
	// Create mock pin handler with default behavior
	pinHandler := NewMockProtocolPinHandler(t).
		WithCreateProtocolPin(func(ctx context.Context, id uint, data any) error {
			return nil
		}).
		WithGetProtocolPin(func(ctx context.Context, tx *gorm.DB, id uint) (any, error) {
			return nil, nil
		}).
		WithUpdateProtocolPin(func(ctx context.Context, id uint, data any) error {
			return nil
		}).
		WithDeleteProtocolPin(func(ctx context.Context, id uint) error {
			return nil
		}).
		WithQueryProtocolPin(func(ctx context.Context, query any) *gorm.DB {
			return nil
		})

	return &MockProtocol{
		tb:               t,
		NameValue:        name,
		DisplayNameValue: name,
		OperationsValue:  []core.Operation{},
		WorkflowsValue:   []core.WorkflowDefinition{},
		PinHandlerValue:  pinHandler,
		HashFunc: func(r io.Reader, size uint64) (core.StorageHash, error) {
			// Read all data from reader
			data, err := io.ReadAll(r)
			if err != nil {
				return nil, err
			}

			// Create SHA2-256 hash of the data
			hash, err := mh.Sum(data, mh.SHA2_256, -1)
			if err != nil {
				return nil, err
			}

			// Create storage hash with CID type 0 (IPFS CIDv0)
			return core.NewStorageHashFromMultihash(hash, 0, nil), nil
		},
	}
}

// WithOperation adds an operation to the mock protocol
func (p *MockProtocol) WithOperation(op core.Operation) *MockProtocol {
	p.OperationsValue = append(p.OperationsValue, op)
	return p
}

// WithConfig sets the config for the mock protocol
func (p *MockProtocol) WithConfig(cfg config.ProtocolConfig) *MockProtocol {
	p.ConfigValue = cfg
	return p
}

// WithPinHandler sets the pin handler for the mock protocol
func (p *MockProtocol) WithPinHandler(handler core.ProtocolPinHandler) *MockProtocol {
	p.PinHandlerValue = handler
	return p
}

func (p *MockProtocol) WithWorkflows(workflows []core.WorkflowDefinition) *MockProtocol {
	p.WorkflowsValue = workflows
	return p
}
