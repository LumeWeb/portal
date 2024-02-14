package account

import (
	"context"
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
}

type AccountAPIParams struct {
	fx.In
	Config      *viper.Viper
	Accounts    *account.AccountServiceDefault
	HttpHandler *HttpHandler
}

func NewS5(params AccountAPIParams) AccountApiResult {
	api := &AccountAPI{
		config:      params.Config,
		accounts:    params.Accounts,
		httpHandler: params.HttpHandler,
	}

	return AccountApiResult{
		API:        api,
		AccountAPI: api,
	}
}

func InitAPI(api *AccountAPI) error {
	return api.Init()
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
	middleware.RegisterProtocolSubdomain(a.config, jape.Mux(getRoutes(a)), "s5")
	return nil
}

func (a AccountAPI) Start(ctx context.Context) error {
	return nil
}

func (a AccountAPI) Stop(ctx context.Context) error {
	return nil
}

func getRoutes(a *AccountAPI) map[string]jape.Handler {
	return map[string]jape.Handler{
		"/api/login":   a.httpHandler.login,
		"api/register": a.httpHandler.register,
	}
}
