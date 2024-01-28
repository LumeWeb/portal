package api

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"git.lumeweb.com/LumeWeb/portal/api/s5"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	protoRegistry "git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API = (*S5API)(nil)
)

type S5API struct {
	config      *viper.Viper
	identity    ed25519.PrivateKey
	accounts    *account.AccountServiceImpl
	storage     *storage.StorageServiceImpl
	protocols   []protoRegistry.Protocol
	httpHandler s5.HttpHandler
	protocol    *protocols.S5Protocol
}

type S5ApiParams struct {
	fx.In
	Config      *viper.Viper
	Identity    ed25519.PrivateKey
	Accounts    *account.AccountServiceImpl
	Storage     *storage.StorageServiceImpl
	Protocols   []protoRegistry.Protocol
	HttpHandler s5.HttpHandler
}

type S5ApiResult struct {
	fx.Out
	Protocol registry.API `group:"api"`
}

func NewS5(params S5ApiParams) (S5ApiResult, error) {
	return S5ApiResult{
		Protocol: &S5API{
			config:      params.Config,
			identity:    params.Identity,
			accounts:    params.Accounts,
			storage:     params.Storage,
			protocols:   params.Protocols,
			httpHandler: params.HttpHandler,
		},
	}, nil
}

var S5Module = fx.Module("s5_api",
	fx.Provide(NewS5),
	fx.Provide(s5.NewHttpHandler),
)

func (s *S5API) Init() error {
	s5protocol := protoRegistry.FindProtocolByName("s5", s.protocols)
	if s5protocol == nil {
		return fmt.Errorf("s5 protocol not found")
	}

	s5protocolInstance := s5protocol.(*protocols.S5Protocol)
	s.protocol = s5protocolInstance
	router := s5protocolInstance.Node().Services().HTTP().GetHttpRouter(getRoutes(s))
	middleware.RegisterProtocolSubdomain(s.config, router, "s5")

	return nil
}

func (s S5API) Name() string {
	return "s5"
}

func (s S5API) Start(ctx context.Context) error {
	return s.protocol.Node().Start()
}

func (s S5API) Stop(ctx context.Context) error {
	return nil
}

func getRoutes(s *S5API) map[string]jape.Handler {
	tusHandler := middleware.BuildS5TusApi(s.identity, s.accounts, s.storage)

	return map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  s.httpHandler.AccountRegisterChallenge,
		"POST /s5/account/register": s.httpHandler.AccountRegister,
		"GET /s5/account/login":     s.httpHandler.AccountLoginChallenge,
		"POST /s5/account/login":    s.httpHandler.AccountLogin,
		"GET /s5/account":           middleware.ApplyMiddlewares(s.httpHandler.AccountInfo, middleware.AuthMiddleware(s.identity, s.accounts)),
		"GET /s5/account/stats":     middleware.ApplyMiddlewares(s.httpHandler.AccountStats, middleware.AuthMiddleware(s.identity, s.accounts)),
		"GET /s5/account/pins.bin":  middleware.ApplyMiddlewares(s.httpHandler.AccountPins, middleware.AuthMiddleware(s.identity, s.accounts)),

		// Upload API
		"POST /s5/upload":           middleware.ApplyMiddlewares(s.httpHandler.SmallFileUpload, middleware.AuthMiddleware(s.identity, s.accounts)),
		"POST /s5/upload/directory": middleware.ApplyMiddlewares(s.httpHandler.DirectoryUpload, middleware.AuthMiddleware(s.identity, s.accounts)),

		// Tus API
		"POST /s5/upload/tus":      tusHandler,
		"HEAD /s5/upload/tus/:id":  tusHandler,
		"POST /s5/upload/tus/:id":  tusHandler,
		"PATCH /s5/upload/tus/:id": tusHandler,

		// Download API
		"GET /s5/blob/:cid":     middleware.ApplyMiddlewares(s.httpHandler.DownloadBlob, middleware.AuthMiddleware(s.identity, s.accounts)),
		"GET /s5/metadata/:cid": s.httpHandler.DownloadMetadata,
		// "GET /s5/download/:cid": middleware.ApplyMiddlewares(s.httpHandler.DownloadFile, middleware.AuthMiddleware(portal)),
		"GET /s5/download/:cid": middleware.ApplyMiddlewares(s.httpHandler.DownloadFile, cors.Default().Handler),

		// Pins API
		"POST /s5/pin/:cid":      middleware.ApplyMiddlewares(s.httpHandler.AccountPin, middleware.AuthMiddleware(s.identity, s.accounts)),
		"DELETE /s5/delete/:cid": middleware.ApplyMiddlewares(s.httpHandler.AccountPinDelete, middleware.AuthMiddleware(s.identity, s.accounts)),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.ApplyMiddlewares(s.httpHandler.DebugDownloadUrls, middleware.AuthMiddleware(s.identity, s.accounts)),
		"GET /s5/debug/storage_locations/:hash": middleware.ApplyMiddlewares(s.httpHandler.DebugStorageLocations, middleware.AuthMiddleware(s.identity, s.accounts)),

		// Registry API
		"GET /s5/registry":              middleware.ApplyMiddlewares(s.httpHandler.RegistryQuery, middleware.AuthMiddleware(s.identity, s.accounts)),
		"POST /s5/registry":             middleware.ApplyMiddlewares(s.httpHandler.RegistrySet, middleware.AuthMiddleware(s.identity, s.accounts)),
		"GET /s5/registry/subscription": middleware.ApplyMiddlewares(s.httpHandler.RegistrySubscription, middleware.AuthMiddleware(s.identity, s.accounts)),
	}
}
