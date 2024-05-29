package service

import (
	"github.com/LumeWeb/portal/core"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strconv"
	"sync"
)

type HTTPServiceDefault struct {
	ctx    *core.Context
	router *mux.Router
	srv    *http.Server
}

var _ handlers.RecoveryHandlerLogger = (*recoverLogger)(nil)

type recoverLogger struct {
	ctx *core.Context
}

func (r *recoverLogger) Println(v ...interface{}) {
	r.ctx.Logger().Error("Recovered from panic", zap.Any("panic", v))
}

func NewHTTPService(ctx *core.Context) *HTTPServiceDefault {
	_http := &HTTPServiceDefault{
		ctx:    ctx,
		router: mux.NewRouter(),
	}

	_http.router.Use(handlers.RecoveryHandler(handlers.RecoveryLogger(&recoverLogger{ctx})))
	ctx.RegisterService(_http)

	srv := &http.Server{
		Addr:    ":" + strconv.FormatUint(uint64(ctx.Config().Config().Core.Port), 10),
		Handler: _http.router,
	}

	ctx.OnExit(func(ctx core.Context) error {
		return srv.Shutdown(ctx)
	})

	_http.srv = srv

	return _http
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
