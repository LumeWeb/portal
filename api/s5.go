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
		"GET /s5/account":           middleware.ApplyMiddlewares(h.AccountInfo, middleware.AuthMiddleware(portal)),
		"GET /s5/account/stats":     middleware.ApplyMiddlewares(h.AccountStats, middleware.AuthMiddleware(portal)),
		"GET /s5/account/pins.bin":  middleware.ApplyMiddlewares(h.AccountPins, middleware.AuthMiddleware(portal)),

		// Upload API
		"POST /s5/upload":           middleware.ApplyMiddlewares(h.SmallFileUpload, middleware.AuthMiddleware(portal)),
		"POST /s5/upload/directory": middleware.ApplyMiddlewares(h.DirectoryUpload, middleware.AuthMiddleware(portal)),

		// Tus API
		"POST /s5/upload/tus":      tusHandler,
		"HEAD /s5/upload/tus/:id":  tusHandler,
		"POST /s5/upload/tus/:id":  tusHandler,
		"PATCH /s5/upload/tus/:id": tusHandler,

		// Download API
		"GET /s5/blob/:cid":     middleware.ApplyMiddlewares(h.DownloadBlob, middleware.AuthMiddleware(portal)),
		"GET /s5/metadata/:cid": h.DownloadMetadata,

		// Pins API
		"POST /s5/pin/:cid":      middleware.ApplyMiddlewares(h.AccountPin, middleware.AuthMiddleware(portal)),
		"DELETE /s5/delete/:cid": middleware.ApplyMiddlewares(h.AccountPinDelete, middleware.AuthMiddleware(portal)),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.ApplyMiddlewares(h.DebugDownloadUrls, middleware.AuthMiddleware(portal)),
		"GET /s5/debug/storage_locations/:hash": middleware.ApplyMiddlewares(h.DebugStorageLocations, middleware.AuthMiddleware(portal)),

		// Registry API
		"GET /s5/registry":              middleware.ApplyMiddlewares(h.RegistryQuery, middleware.AuthMiddleware(portal)),
		"POST /s5/registry":             middleware.ApplyMiddlewares(h.RegistrySet, middleware.AuthMiddleware(portal)),
		"GET /s5/registry/subscription": middleware.ApplyMiddlewares(h.RegistrySubscription, middleware.AuthMiddleware(portal)),
	}
}
