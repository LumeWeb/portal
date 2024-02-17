//go:build s5

package protocols

import (
	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/protocols/s5"
)

func init() {
	registry.Register(registry.ProtocolEntry{
		Key:         "s5",
		Module:      s5.ProtocolModule,
		PreInitFunc: s5.PreInit,
	})
}
