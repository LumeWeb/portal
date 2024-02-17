//go:build s5

package api

import (
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"git.lumeweb.com/LumeWeb/portal/api/s5"
)

func init() {
	registry.Register(registry.APIEntry{
		Key:    "s5",
		Module: s5.Module,
	})
}
