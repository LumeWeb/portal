package sync

import (
	"context"
	"encoding/hex"
	"net/http"

	"github.com/LumeWeb/portal/sync"

	"github.com/LumeWeb/portal/config"

	"github.com/LumeWeb/portal/account"

	"github.com/LumeWeb/portal/api/middleware"
	"github.com/LumeWeb/portal/api/registry"
	"github.com/LumeWeb/portal/api/router"
	"github.com/julienschmidt/httprouter"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API       = (*SyncAPI)(nil)
	_ router.RoutableAPI = (*SyncAPI)(nil)
)

type SyncAPI struct {
	config      *config.Manager
	syncService *sync.SyncServiceDefault
}

func (s *SyncAPI) Init() error {
	return nil
}

func (s *SyncAPI) Start(ctx context.Context) error {
	return nil
}

func (s *SyncAPI) Stop(ctx context.Context) error {
	return nil
}

func (s *SyncAPI) Name() string {
	return "sync"
}

func (s *SyncAPI) Domain() string {
	return router.BuildSubdomain(s, s.config)
}

func (s *SyncAPI) AuthTokenName() string {
	return account.AUTH_COOKIE_NAME
}

func (s *SyncAPI) Can(w http.ResponseWriter, r *http.Request) bool {
	return false
}

func (s *SyncAPI) Handle(w http.ResponseWriter, r *http.Request) {

}

func (s *SyncAPI) Routes() (*httprouter.Router, error) {
	routes := map[string]jape.Handler{
		"GET /api/log/key": middleware.ApplyMiddlewares(s.logKey, middleware.ProxyMiddleware),
	}

	return jape.Mux(routes), nil
}

func (s *SyncAPI) logKey(jc jape.Context) {
	keyHex := hex.EncodeToString(s.syncService.LogKey())

	jc.Encode(&LogKeyResponse{
		Key: keyHex,
	})
}

type SyncApiResult struct {
	fx.Out
	API         registry.API `group:"api"`
	SyncAPI     *SyncAPI
	SyncService *sync.SyncServiceDefault
}

type APIParams struct {
	fx.In
	Config      *config.Manager
	SyncService *sync.SyncServiceDefault
}

func NewSync(params APIParams) SyncApiResult {
	api := &SyncAPI{config: params.Config, syncService: params.SyncService}

	return SyncApiResult{
		API:     api,
		SyncAPI: api,
	}
}

var Module = fx.Module("sync_api",
	fx.Provide(NewSync),
)
