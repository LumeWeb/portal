package service

import (
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.lumeweb.com/httputil"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/middleware"
	"go.uber.org/zap"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"sync"
)

var _ core.HTTPService = (*HTTPServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.HTTP_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewHTTPService()
		},
	})
}

type HTTPServiceDefault struct {
	ctx    core.Context
	logger *core.Logger
	router *mux.Router
	srv    *http.Server
}

var _ handlers.RecoveryHandlerLogger = (*recoverLogger)(nil)

type recoverLogger struct {
	ctx core.Context
}

func (r *recoverLogger) Println(v ...interface{}) {
	r.ctx.Logger().Error("Recovered from panic", zap.Any("panic", v))
}

func NewHTTPService() (*HTTPServiceDefault, []core.ContextBuilderOption, error) {
	_http := &HTTPServiceDefault{
		router: mux.NewRouter(),
	}

	srv := &http.Server{
		Handler: _http.router,
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_http.ctx = ctx
			_http.logger = ctx.ServiceLogger(_http)
			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return srv.Shutdown(ctx)
		}),
	)

	_http.srv = srv

	return _http, opts, nil
}

func (h *HTTPServiceDefault) ID() string {
	return core.HTTP_SERVICE
}

func (h *HTTPServiceDefault) Router() *mux.Router {
	return h.router
}

func (h *HTTPServiceDefault) Init() error {
	h.router.Use(handlers.RecoveryHandler(handlers.RecoveryLogger(&recoverLogger{h.ctx})))
	h.srv.Addr = ":" + strconv.FormatUint(uint64(h.ctx.Config().Config().Core.Port), 10)
	for _, api := range core.GetAPIs() {
		domain := fmt.Sprintf("%s.%s", api.Subdomain(), h.ctx.Config().Config().Core.Domain)
		err := api.Configure(h.Router().Host(domain).Subrouter())
		if err != nil {
			return err
		}
	}

	authMw := middleware.AuthMiddleware(middleware.AuthMiddlewareOptions{
		Context: h.ctx,
		Purpose: core.JWTPurposeLogin,
	})

	h.Router().PathPrefix("/debug/").Handler(http.DefaultServeMux).Use(authMw)

	corsOpts := cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}
	corsHandler := cors.New(corsOpts)

	rootApi := h.Router().PathPrefix("/api").Subrouter()
	rootApi.Use(corsHandler.Handler)
	rootApi.HandleFunc("/meta", h.apiMetaHandler).Methods(http.MethodGet)

	return nil
}

func (h *HTTPServiceDefault) apiMetaHandler(w http.ResponseWriter, r *http.Request) {
	ctx := httputil.Context(r, w)

	metaBuilder := NewPortalMetaBuilder(h.ctx.Config().Config().Core.Domain)

	for _, plugin := range core.GetPlugins() {
		metaBuilder.AddPlugin(plugin.ID)
	}

	for _, plugin := range core.GetPlugins() {
		if plugin.Meta != nil {
			err := plugin.Meta(h.ctx, metaBuilder)
			if err != nil {
				http.Error(w, "Failed to build meta", http.StatusInternalServerError)
				h.logger.Error("Failed to build meta", zap.Error(err))

				return
			}
		}
	}

	ctx.Encode(metaBuilder.Build())
}

func (h *HTTPServiceDefault) Serve() error {
	wg := sync.WaitGroup{}
	wg.Add(1)

	ln, err := net.Listen("tcp", h.srv.Addr)
	if err != nil {
		return err
	}

	go func() {
		defer wg.Done()
		err := h.srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			h.logger.Fatal("Failed to serve", zap.Error(err))
		}
	}()

	wg.Wait()
	return nil
}
