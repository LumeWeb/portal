package registry

import (
	"context"

	"github.com/LumeWeb/portal/config"

	"go.uber.org/fx"
)

type Protocol interface {
	Name() string
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Config() config.ProtocolConfig
}

type ProtocolEntry struct {
	Key         string
	Module      fx.Option
	PreInitFunc interface{}
}

var protocolEntryRegistry []ProtocolEntry
var protocolRegistry map[string]Protocol

func init() {
	protocolRegistry = make(map[string]Protocol)
}

func RegisterEntry(entry ProtocolEntry) {
	protocolEntryRegistry = append(protocolEntryRegistry, entry)
}

func RegisterProtocol(protocol Protocol) {
	protocolRegistry[protocol.Name()] = protocol
}

func GetEntryRegistry() []ProtocolEntry {
	return protocolEntryRegistry
}

func GetProtocol(name string) Protocol {
	if _, ok := protocolRegistry[name]; !ok {
		return nil
	}

	return protocolRegistry[name]
}

func GetAllProtocols() map[string]Protocol {
	return protocolRegistry
}
