package api

import (
	"github.com/LumeWeb/portal/api/registry"
	"github.com/LumeWeb/portal/api/sync"
)

func init() {
	registry.RegisterEntry(registry.APIEntry{
		Key:    "sync",
		Module: sync.Module,
	})
}
