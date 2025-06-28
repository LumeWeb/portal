package core

import (
	"context"
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/db/models/data_models"
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
	Operations() []Operation
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
	GetProtocolData(ctx context.Context, tx *gorm.DB, id uint) (any, error)
	UpdateProtocolData(ctx context.Context, id uint, data any) error
	DeleteProtocolData(ctx context.Context, id uint) error
	QueryProtocolData(ctx context.Context, tx *gorm.DB, query any) *gorm.DB
	CompleteProtocolData(ctx context.Context, id uint) error
	GetProtocolDataModel() any
}

type ProtocolPinHandler interface {
	CreateProtocolPin(ctx context.Context, id uint, data any) error
	GetProtocolPin(ctx context.Context, tx *gorm.DB, id uint) (any, error)
	UpdateProtocolPin(ctx context.Context, id uint, data any) error
	DeleteProtocolPin(ctx context.Context, id uint) error
	QueryProtocolPin(ctx context.Context, query any) *gorm.DB
	GetProtocolPinModel() data_models.PinDataModel
}

type ProtocolGetPinHandler interface {
	PinHandler() ProtocolPinHandler
}

type TestingProtocolRequestDataHandler interface {
	Protocol
	ProtocolRequestDataHandler
}

// TestingProtocolPinHandler is a composite interface for testing
// protocols that also handle pins.
type TestingProtocolPinHandler interface {
	Protocol
	ProtocolPinHandler
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

func ResetProtocols() {
	protocolsMu.Lock()
	defer protocolsMu.Unlock()
	protocols = make(map[string]Protocol)
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

	if _, ok := protocol.(ProtocolPinHandler); ok {
		return true
	}

	if getter, ok := protocol.(ProtocolGetPinHandler); ok {
		return getter.PinHandler() != nil
	}

	return false
}

func GetProtocolPinHandler(name string) ProtocolPinHandler {
	protocol, ok := protocols[name]

	if !ok {
		panic(fmt.Sprintf("protocol not found: %s", name))
	}

	if handler, ok := protocol.(ProtocolPinHandler); ok {
		return handler
	}

	if getter, ok := protocol.(ProtocolGetPinHandler); ok {
		if handler := getter.PinHandler(); handler != nil {
			return handler
		}
	}

	panic(fmt.Sprintf("protocol does not have a data pin handler: %T", protocol))
}
