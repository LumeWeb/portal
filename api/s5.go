//go:build s5

package api

import (
	"github.com/LumeWeb/portal/api/registry"
	"github.com/LumeWeb/portal/api/s5"
)

func init() {
	registry.RegisterEntry(registry.APIEntry{
		Key:    "s5",
		Module: s5.Module,
	})
}
