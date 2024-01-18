package api

import (
	"git.lumeweb.com/LumeWeb/portal/api/s5"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"go.sia.tech/jape"
)

var (
	_ interfaces.API = (*S5API)(nil)
)

type S5API struct {
}

func NewS5() *S5API {
	return &S5API{}
}

func (s S5API) Initialize(portal interfaces.Portal, protocol interfaces.Protocol) error {
	s5protocol := protocol.(*protocols.S5Protocol)
	s5http := s5.NewHttpHandler(portal)
	registerProtocolSubdomain(portal, s5protocol.Node().Services().HTTP().GetHttpRouter(getRoutes(s5http, portal)), "s5")

	return nil
}

func getRoutes(h *s5.HttpHandler, portal interfaces.Portal) map[string]jape.Handler {
	return map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  h.AccountRegisterChallenge,
		"POST /s5/account/register": h.AccountRegister,
		"GET /s5/account/login":     h.AccountLoginChallenge,
		"POST /s5/account/login":    h.AccountLogin,
		"GET /s5/account":           s5.AuthMiddleware(h.AccountInfo, portal),
		"GET /s5/account/stats":     s5.AuthMiddleware(h.AccountStats, portal),
		"GET /s5/account/pins.bin":  s5.AuthMiddleware(h.AccountPins, portal),

		// Upload API
		"POST /s5/upload":           s5.AuthMiddleware(h.SmallFileUpload, portal),
		"POST /s5/upload/directory": s5.AuthMiddleware(h.DirectoryUpload, portal),

		// Download API
		"GET /s5/blob/:cid":     s5.AuthMiddleware(h.DownloadBlob, portal),
		"GET /s5/metadata/:cid": s5.AuthMiddleware(h.DownloadMetadata, portal),

		// Pins API
		"POST /s5/pin/:cid":      s5.AuthMiddleware(h.AccountPin, portal),
		"DELETE /s5/delete/:cid": s5.AuthMiddleware(h.AccountPinDelete, portal),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      s5.AuthMiddleware(h.DebugDownloadUrls, portal),
		"GET /s5/debug/storage_locations/:hash": s5.AuthMiddleware(h.DebugStorageLocations, portal),

		//Registry API
		"GET /s5/registry":              s5.AuthMiddleware(h.RegistryQuery, portal),
		"POST /s5/registry":             s5.AuthMiddleware(h.RegistrySet, portal),
		"GET /s5/registry/subscription": s5.AuthMiddleware(h.RegistrySubscription, portal),
	}
}
