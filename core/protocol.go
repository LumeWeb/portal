package core

import (
	"fmt"
	"go.lumeweb.com/portal/config"
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
