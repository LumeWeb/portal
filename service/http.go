package service

import (
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
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
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_http.router.Use(handlers.RecoveryHandler(handlers.RecoveryLogger(&recoverLogger{ctx})))
			srv.Addr = ":" + strconv.FormatUint(uint64(ctx.Config().Config().Core.Port), 10)
			for _, api := range core.GetAPIs() {
				domain := fmt.Sprintf("%s.%s", api.Subdomain(), ctx.Config().Config().Core.Domain)
				err := api.Configure(_http.Router().Host(domain).Subrouter())
				if err != nil {
					return err
				}
			}

			authMw := middleware.AuthMiddleware(middleware.AuthMiddlewareOptions{
				Context: ctx,
				Purpose: core.JWTPurposeLogin,
			})

			_http.Router().PathPrefix("/debug/").Handler(http.DefaultServeMux).Use(authMw)

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return srv.Shutdown(ctx)
		}),
	)

	_http.srv = srv

	return _http, opts, nil
}

func (h *HTTPServiceDefault) Router() *mux.Router {
	return h.router
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
		if err != nil {
			h.ctx.Logger().Fatal("Failed to serve", zap.Error(err))
		}
	}()

	wg.Wait()
	return nil
}
