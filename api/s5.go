package api

import (
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
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

	tusHandler := middleware.BuildS5TusApi(portal)

	return map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  h.AccountRegisterChallenge,
		"POST /s5/account/register": h.AccountRegister,
		"GET /s5/account/login":     h.AccountLoginChallenge,
		"POST /s5/account/login":    h.AccountLogin,
		"GET /s5/account":           middleware.AuthMiddleware(h.AccountInfo, portal),
		"GET /s5/account/stats":     middleware.AuthMiddleware(h.AccountStats, portal),
		"GET /s5/account/pins.bin":  middleware.AuthMiddleware(h.AccountPins, portal),

		// Upload API
		"POST /s5/upload":           middleware.AuthMiddleware(h.SmallFileUpload, portal),
		"POST /s5/upload/directory": middleware.AuthMiddleware(h.DirectoryUpload, portal),

		// Download API
		"GET /s5/blob/:cid":     middleware.AuthMiddleware(h.DownloadBlob, portal),
		"GET /s5/metadata/:cid": h.DownloadMetadata,

		// Pins API
		"POST /s5/pin/:cid":      middleware.AuthMiddleware(h.AccountPin, portal),
		"DELETE /s5/delete/:cid": middleware.AuthMiddleware(h.AccountPinDelete, portal),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.AuthMiddleware(h.DebugDownloadUrls, portal),
		"GET /s5/debug/storage_locations/:hash": middleware.AuthMiddleware(h.DebugStorageLocations, portal),

		// Registry API
		"GET /s5/registry":              middleware.AuthMiddleware(h.RegistryQuery, portal),
		"POST /s5/registry":             middleware.AuthMiddleware(h.RegistrySet, portal),
		"GET /s5/registry/subscription": middleware.AuthMiddleware(h.RegistrySubscription, portal),
	}
}
