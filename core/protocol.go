package core

import (
	"context"
	"fmt"
	"go.lumeweb.com/portal/config"
	"gorm.io/gorm"
	"sort"
	"sync"
)

var (
	protocols   = make(map[string]Protocol)
	protocolsMu sync.RWMutex
)

type Protocol interface {
	Name() string
	Config() config.ProtocolConfig
}

type ProtocolInit interface {
	Init(ctx Context) error
}

type ProtocolStart interface {
	Start(ctx Context) error
}

type ProtocolStop interface {
	Stop(ctx Context) error
}

type ProtocolRequestDataHandler interface {
	CreateProtocolData(ctx context.Context, id uint, data any) error
	GetProtocolData(ctx context.Context, id uint) (any, error)
	UpdateProtocolData(ctx context.Context, id uint, data any) error
	DeleteProtocolData(ctx context.Context, id uint) error
	QueryProtocolData(ctx context.Context, tx *gorm.DB, query any) *gorm.DB
	CompleteProtocolData(ctx context.Context, id uint) error
	GetProtocolDataModel() any
}

type ProtocolPinHandler interface {
	CreateProtocolPin(ctx context.Context, id uint, data any) error
	GetProtocolPin(ctx context.Context, id uint) (any, error)
	UpdateProtocolPin(ctx context.Context, id uint, data any) error
	DeleteProtocolPin(ctx context.Context, id uint) error
	QueryProtocolPin(ctx context.Context, query any) *gorm.DB
	GetProtocolPinModel() any
}

func RegisterProtocol(id string, protocol Protocol) {
	protocolsMu.Lock()
	defer protocolsMu.Unlock()

	if _, ok := protocols[id]; ok {
		panic(fmt.Sprintf("protocol already registered: %s", id))
	}

	protocols[id] = protocol
}

func GetProtocols() map[string]Protocol {
	apisMu.RLock()
	defer apisMu.RUnlock()

	return protocols
}

func GetProtocolList() []Protocol {
	protocolsMu.RLock()
	defer protocolsMu.RUnlock()

	keys := make([]string, 0, len(protocols))
	for k := range protocols {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var protocolList []Protocol
	for _, k := range keys {
		protocolList = append(protocolList, protocols[k])
	}

	return protocolList
}

func PluginHasProtocol(plugin PluginInfo) bool {
	return plugin.Protocol != nil
}

func ProtocolHasDataRequestHandler(name string) bool {
	protocol, ok := protocols[name]

	if !ok {
		return false
	}

	_, ok = protocol.(ProtocolRequestDataHandler)
	return ok
}

func GetProtocolDataRequestHandler(name string) ProtocolRequestDataHandler {
	protocol, ok := protocols[name]

	if !ok {
		panic(fmt.Sprintf("protocol not found: %s", name))
	}

	handler, ok := protocol.(ProtocolRequestDataHandler)
	if !ok {
		panic(fmt.Sprintf("protocol does not have a request handler: %T", protocol))
	}

	return handler
}

func ProtocolHasPinHandler(name string) bool {
	protocol, ok := protocols[name]

	if !ok {
		return false
	}

	_, ok = protocol.(ProtocolPinHandler)
	return ok
}

func GetProtocolPinHandler(name string) ProtocolPinHandler {
	protocol, ok := protocols[name]

	if !ok {
		panic(fmt.Sprintf("protocol not found: %s", name))
	}

	handler, ok := protocol.(ProtocolPinHandler)
	if !ok {
		panic(fmt.Sprintf("protocol does not have a data pin handler: %T", protocol))
	}

	return handler
}
