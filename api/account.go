package api

import (
	"github.com/LumeWeb/portal/api/account"
	"github.com/LumeWeb/portal/api/registry"
)

func init() {
	registry.RegisterEntry(registry.APIEntry{
		Key:    "account",
		Module: account.Module,
	})
}
