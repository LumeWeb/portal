//go:build s5

package protocols

import (
	"github.com/LumeWeb/portal/protocols/registry"
	"github.com/LumeWeb/portal/protocols/s5"
)

func init() {
	registry.RegisterEntry(registry.ProtocolEntry{
		Key:         "s5",
		Module:      s5.ProtocolModule,
		PreInitFunc: s5.PreInit,
	})
}
