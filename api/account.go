package api

import (
	"git.lumeweb.com/LumeWeb/portal/api/account"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
)

func init() {
	registry.RegisterEntry(registry.APIEntry{
		Key:    "account",
		Module: account.Module,
	})
}
