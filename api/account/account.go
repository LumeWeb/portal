package account

import (
	"context"
	"crypto/ed25519"

	"github.com/julienschmidt/httprouter"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API = (*AccountAPI)(nil)
)

type AccountAPI struct {
	config      *viper.Viper
	accounts    *account.AccountServiceDefault
	httpHandler *HttpHandler
	identity    ed25519.PrivateKey
}

type AccountAPIParams struct {
	fx.In
	Config      *viper.Viper
	Accounts    *account.AccountServiceDefault
	HttpHandler *HttpHandler
	Identity    ed25519.PrivateKey
}

func NewS5(params AccountAPIParams) AccountApiResult {
	api := &AccountAPI{
		config:      params.Config,
		accounts:    params.Accounts,
		httpHandler: params.HttpHandler,
		identity:    params.Identity,
	}

	return AccountApiResult{
		API:        api,
		AccountAPI: api,
	}
}

var Module = fx.Module("s5_api",
	fx.Provide(NewS5),
	fx.Provide(NewHttpHandler),
)

type AccountApiResult struct {
	fx.Out
	API        registry.API `group:"api"`
	AccountAPI *AccountAPI
}

func (a AccountAPI) Name() string {
	return "account"
}

func (a *AccountAPI) Init() error {
	return nil
}

func (a AccountAPI) Start(ctx context.Context) error {
	return nil
}

func (a AccountAPI) Stop(ctx context.Context) error {
	return nil
}

func (a AccountAPI) Routes() *httprouter.Router {
	authMw2fa := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurpose2FA,
	})

	authMw := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurposeLogin,
	})

	return jape.Mux(map[string]jape.Handler{
		"/api/auth/login":        middleware.ApplyMiddlewares(a.httpHandler.login, authMw2fa, middleware.ProxyMiddleware),
		"/api/auth/register":     a.httpHandler.register,
		"/api/auth/otp/generate": middleware.ApplyMiddlewares(a.httpHandler.otpGenerate, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/verify":   middleware.ApplyMiddlewares(a.httpHandler.otpVerify, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/validate": middleware.ApplyMiddlewares(a.httpHandler.otpValidate, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/disable":  middleware.ApplyMiddlewares(a.httpHandler.otpDisable, authMw, middleware.ProxyMiddleware),
	})
}
