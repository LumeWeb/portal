package core

import (
	"go.sia.tech/core/types"
	sdk "go.sia.tech/siastorage"
)

// PortalAppMetadata returns the app metadata used to register the portal
// with the Sia indexer. This is the single source of truth — both the
// RenterService (startup) and the `portal sia login` CLI command use it.
//
// The app ID is derived from a fixed string; changing it invalidates
// any existing app keys.
func PortalAppMetadata() sdk.AppMetadata {
	return sdk.AppMetadata{
		ID:          types.HashBytes([]byte("lumeweb-portal")),
		Name:        "LumeWeb Portal",
		Description: "A decentralized storage and content delivery portal",
		ServiceURL:  "https://lumeweb.com",
	}
}
