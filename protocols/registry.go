package protocols

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
)

var (
	_ interfaces.ProtocolRegistry = (*ProtocolRegistryImpl)(nil)
)

type ProtocolRegistryImpl struct {
	protocols map[string]interfaces.Protocol
}

func NewProtocolRegistry() interfaces.ProtocolRegistry {
	return &ProtocolRegistryImpl{
		protocols: make(map[string]interfaces.Protocol),
	}
}

func (r *ProtocolRegistryImpl) Register(name string, protocol interfaces.Protocol) {
	if _, exists := r.protocols[name]; exists {
		panic("protocol already registered")
	}
	r.protocols[name] = protocol
}

func (r *ProtocolRegistryImpl) Get(name string) (interfaces.Protocol, error) {
	protocol, exists := r.protocols[name]
	if !exists {
		return nil, errors.New("protocol not found")
	}
	return protocol, nil
}

func (r *ProtocolRegistryImpl) All() map[string]interfaces.Protocol {
	pMap := make(map[string]interfaces.Protocol)
	for key, value := range r.protocols {
		pMap[key] = value
	}
	return pMap
}
