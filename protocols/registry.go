package protocols

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
)

var (
	_ ProtocolRegistry = (*ProtocolRegistryImpl)(nil)
)

type ProtocolRegistry interface {
	Register(name string, protocol interfaces.Protocol) error
	Get(name string) (interfaces.Protocol, error)
}

type ProtocolRegistryImpl struct {
	protocols map[string]interfaces.Protocol
}

func NewProtocolRegistry() ProtocolRegistry {
	return &ProtocolRegistryImpl{
		protocols: make(map[string]interfaces.Protocol),
	}
}

func (r *ProtocolRegistryImpl) Register(name string, protocol interfaces.Protocol) error {
	if _, exists := r.protocols[name]; exists {
		return errors.New("protocol already registered")
	}
	r.protocols[name] = protocol
	return nil
}

func (r *ProtocolRegistryImpl) Get(name string) (interfaces.Protocol, error) {
	protocol, exists := r.protocols[name]
	if !exists {
		return nil, errors.New("protocol not found")
	}
	return protocol, nil
}
