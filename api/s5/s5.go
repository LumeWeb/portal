package s5

import (
	"context"
	"crypto/ed25519"
	"embed"
	_ "embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"

	"github.com/julienschmidt/httprouter"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	protoRegistry "git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/protocols/s5"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API = (*S5API)(nil)
)

//go:embed swagger.yaml
var spec []byte

//go:generate go run generate.go

//go:embed embed
var swagfs embed.FS

type S5API struct {
	config      *viper.Viper
	identity    ed25519.PrivateKey
	accounts    *account.AccountServiceDefault
	storage     *storage.StorageServiceDefault
	protocols   []protoRegistry.Protocol
	httpHandler HttpHandler
	protocol    *s5.S5Protocol
}

type APIParams struct {
	fx.In
	Config      *viper.Viper
	Identity    ed25519.PrivateKey
	Accounts    *account.AccountServiceDefault
	Storage     *storage.StorageServiceDefault
	Protocols   []protoRegistry.Protocol `group:"protocol"`
	HttpHandler HttpHandler
}

type S5ApiResult struct {
	fx.Out
	API   registry.API `group:"api"`
	S5API *S5API
}

func NewS5(params APIParams) (S5ApiResult, error) {
	api := &S5API{
		config:      params.Config,
		identity:    params.Identity,
		accounts:    params.Accounts,
		storage:     params.Storage,
		protocols:   params.Protocols,
		httpHandler: params.HttpHandler,
	}
	return S5ApiResult{
		API:   api,
		S5API: api,
	}, nil
}

var Module = fx.Module("s5_api",
	fx.Provide(NewS5),
	fx.Provide(NewHttpHandler),
)

func (s *S5API) Init() error {
	s5protocol := protoRegistry.FindProtocolByName("s5", s.protocols)
	if s5protocol == nil {
		return fmt.Errorf("s5 protocol not found")
	}

	s5protocolInstance := s5protocol.(*s5.S5Protocol)
	s.protocol = s5protocolInstance

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

func (s *S5API) Routes() *httprouter.Router {
	authMiddlewareOpts := middleware.AuthMiddlewareOptions{
		Identity: s.identity,
		Accounts: s.accounts,
		Config:   s.config,
		Purpose:  account.JWTPurposeLogin,
	}

	authMw := authMiddleware(authMiddlewareOpts)

	tusHandler := BuildS5TusApi(authMw, s.storage)

	tusOptionsHandler := func(c jape.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	}

	tusCors := BuildTusCors()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(spec)
	if err != nil {
		panic(err)
	}

	if err = doc.Validate(loader.Context); err != nil {
		panic(err)
	}

	jsonDoc, err := doc.MarshalJSON()

	if err != nil {
		panic(err)
	}

	wrappedTusHandler := middleware.ApplyMiddlewares(tusOptionsHandler, tusCors, authMw)

	swaggerFiles, _ := fs.Sub(swagfs, "embed")
	swaggerServ := http.FileServer(http.FS(swaggerFiles))
	swaggerHandler := func(c jape.Context) {
		swaggerServ.ServeHTTP(c.ResponseWriter, c.Request)
	}

	swaggerStrip := func(next http.Handler) http.Handler {
		return http.StripPrefix("/swagger", next)
	}

	swaggerRedirect := func(jc jape.Context) {
		http.Redirect(jc.ResponseWriter, jc.Request, "/swagger/", http.StatusMovedPermanently)
	}

	routes := map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  s.httpHandler.accountRegisterChallenge,
		"POST /s5/account/register": s.httpHandler.accountRegister,
		"GET /s5/account/login":     s.httpHandler.accountLoginChallenge,
		"POST /s5/account/login":    s.httpHandler.accountLogin,
		"GET /s5/account":           middleware.ApplyMiddlewares(s.httpHandler.accountInfo, authMw),
		"GET /s5/account/stats":     middleware.ApplyMiddlewares(s.httpHandler.accountStats, authMw),
		"GET /s5/account/pins.bin":  middleware.ApplyMiddlewares(s.httpHandler.accountPins, authMw),

		// Upload API
		"POST /s5/upload":           middleware.ApplyMiddlewares(s.httpHandler.smallFileUpload, authMw),
		"POST /s5/upload/directory": middleware.ApplyMiddlewares(s.httpHandler.directoryUpload, authMw),

		// Tus API
		"POST /s5/upload/tus":        tusHandler,
		"OPTIONS /s5/upload/tus":     wrappedTusHandler,
		"HEAD /s5/upload/tus/:id":    tusHandler,
		"POST /s5/upload/tus/:id":    tusHandler,
		"PATCH /s5/upload/tus/:id":   tusHandler,
		"OPTIONS /s5/upload/tus/:id": wrappedTusHandler,

		// Download API
		"GET /s5/blob/:cid":     middleware.ApplyMiddlewares(s.httpHandler.downloadBlob, authMw),
		"GET /s5/metadata/:cid": s.httpHandler.downloadMetadata,
		"GET /s5/download/:cid": middleware.ApplyMiddlewares(s.httpHandler.downloadFile, cors.Default().Handler),

		// Pins API
		"POST /s5/pin/:cid":      middleware.ApplyMiddlewares(s.httpHandler.accountPin, authMw),
		"DELETE /s5/delete/:cid": middleware.ApplyMiddlewares(s.httpHandler.accountPinDelete, authMw),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.ApplyMiddlewares(s.httpHandler.debugDownloadUrls, authMw),
		"GET /s5/debug/storage_locations/:hash": middleware.ApplyMiddlewares(s.httpHandler.debugStorageLocations, authMw),

		// Registry API
		"GET /s5/registry":              middleware.ApplyMiddlewares(s.httpHandler.registryQuery, authMw),
		"POST /s5/registry":             middleware.ApplyMiddlewares(s.httpHandler.registrySet, authMw),
		"GET /s5/registry/subscription": middleware.ApplyMiddlewares(s.httpHandler.registrySubscription, authMw),

		"GET /swagger.json":  byteHandler(jsonDoc),
		"GET /swagger":       swaggerRedirect,
		"GET /swagger/*path": middleware.ApplyMiddlewares(swaggerHandler, swaggerStrip),
	}

	return s.protocol.Node().Services().HTTP().GetHttpRouter(routes)
}

func byteHandler(b []byte) jape.Handler {
	return func(c jape.Context) {
		c.ResponseWriter.Header().Set("Content-Type", "application/json")
		c.ResponseWriter.Write(b)
	}
}

type s5TusJwtResponseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (w *s5TusJwtResponseWriter) WriteHeader(statusCode int) {
	// Check if this is the specific route and status
	if statusCode == http.StatusCreated {
		location := w.Header().Get("Location")
		authToken := middleware.ParseAuthTokenHeader(w.req.Header)

		if authToken != "" && location != "" {

			parsedUrl, _ := url.Parse(location)

			query := parsedUrl.Query()
			query.Set("auth_token", authToken)
			parsedUrl.RawQuery = query.Encode()

			w.Header().Set("Location", parsedUrl.String())
		}
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

func BuildTusCors() func(h http.Handler) http.Handler {
	mw :=
		cors.New(cors.Options{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders: []string{
				"Authorization",
				"Expires",
				"Upload-Concat",
				"Upload-Length",
				"Upload-Offset",
				"X-Requested-With",
				"Tus-Version",
				"Tus-Resumable",
				"Tus-Extension",
				"Tus-Max-Size",
				"X-HTTP-Method-Override",
			},
			AllowCredentials: true,
		})

	return mw.Handler
}

func BuildS5TusApi(authMw middleware.HttpMiddlewareFunc, storage *storage.StorageServiceDefault) jape.Handler {
	// Create a jape.Handler for your tusHandler
	tusJapeHandler := func(c jape.Context) {
		tusHandler := storage.Tus()
		tusHandler.ServeHTTP(c.ResponseWriter, c.Request)
	}

	protocolMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "protocol", "s5")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	stripPrefix := func(next http.Handler) http.Handler {
		return http.StripPrefix("/s5/upload/tus", next)
	}

	injectJwt := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := w
			if r.Method == http.MethodPost && r.URL.Path == "/s5/upload/tus" {
				res = &s5TusJwtResponseWriter{ResponseWriter: w, req: r}
			}

			next.ServeHTTP(res, r)
		})
	}

	// Apply the middlewares to the tusJapeHandler
	tusHandler := middleware.ApplyMiddlewares(tusJapeHandler, BuildTusCors(), authMw, injectJwt, protocolMiddleware, stripPrefix, middleware.ProxyMiddleware)

	return tusHandler
}
