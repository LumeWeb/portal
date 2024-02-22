package registry

import (
	"context"

	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/fx"
)

const GroupName = "protocols"

type Protocol interface {
	Name() string
	Init() error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Config() config.ProtocolConfig
}

type ProtocolEntry struct {
	Key         string
	Module      fx.Option
	PreInitFunc interface{}
}

var protocolEntry []ProtocolEntry

func Register(entry ProtocolEntry) {
	protocolEntry = append(protocolEntry, entry)
}

func GetRegistry() []ProtocolEntry {
	return protocolEntry
}

func FindProtocolByName(name string, protocols []Protocol) Protocol {
	for _, protocol := range protocols {
		if protocol.Name() == name {
			return protocol
		}
	}
	return nil
}
