package sync

import (
	"context"
	"crypto/ed25519"
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
	identity    ed25519.PrivateKey
	accounts    *account.AccountServiceDefault
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

	authMiddlewareOpts := middleware.AuthMiddlewareOptions{
		Identity: s.identity,
		Accounts: s.accounts,
		Config:   s.config,
		Purpose:  account.JWTPurposeLogin,
	}

	authMw := middleware.AuthMiddleware(authMiddlewareOpts)

	routes := map[string]jape.Handler{
		"GET /api/log/key": middleware.ApplyMiddlewares(s.logKey, middleware.ProxyMiddleware, authMw),
		"POST /api/import": middleware.ApplyMiddlewares(s.objectImport, middleware.ProxyMiddleware, authMw),
	}

	return jape.Mux(routes), nil
}

func (s *SyncAPI) logKey(jc jape.Context) {
	keyHex := hex.EncodeToString(s.syncService.LogKey())

	jc.Encode(&LogKeyResponse{
		Key: keyHex,
	})
}

func (s *SyncAPI) objectImport(jc jape.Context) {
	var req ObjectImportRequest

	if err := jc.Decode(&req); err != nil {
		return
	}

	user := middleware.GetUserFromContext(jc.Request.Context())

	err := s.syncService.Import(req.Object, uint64(user))

	if err != nil {
		_ = jc.Error(err, http.StatusBadRequest)
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
}

type SyncApiResult struct {
	fx.Out
	API     registry.API `group:"api"`
	SyncAPI *SyncAPI
}

type APIParams struct {
	fx.In
	Config      *config.Manager
	SyncService *sync.SyncServiceDefault
	Identity    ed25519.PrivateKey
	Accounts    *account.AccountServiceDefault
}

func NewSync(params APIParams) SyncApiResult {
	api := &SyncAPI{
		config:      params.Config,
		syncService: params.SyncService,
		identity:    params.Identity,
		accounts:    params.Accounts,
	}

	return SyncApiResult{
		API:     api,
		SyncAPI: api,
	}
}

var Module = fx.Module("sync_api",
	fx.Provide(NewSync),
)
