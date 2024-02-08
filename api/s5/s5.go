package s5

import (
	"context"
	"crypto/ed25519"
	"embed"
	_ "embed"
	"fmt"
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
	"io/fs"
	"net/http"
	"net/url"
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

func InitAPI(api *S5API) error {
	return api.Init()
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

func byteHandler(b []byte) jape.Handler {
	return func(c jape.Context) {
		c.ResponseWriter.Write(b)
	}
}

func getRoutes(s *S5API) map[string]jape.Handler {
	tusHandler := BuildS5TusApi(s.identity, s.accounts, s.storage)

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

	wrappedTusHandler := middleware.ApplyMiddlewares(tusOptionsHandler, tusCors, middleware.AuthMiddleware(s.identity, s.accounts))

	swaggerFiles, _ := fs.Sub(swagfs, "embed")
	swaggerServ := http.FileServer(http.FS(swaggerFiles))
	swaggerHandler := func(c jape.Context) {
		swaggerServ.ServeHTTP(c.ResponseWriter, c.Request)
	}

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
		"POST /s5/upload/tus":        tusHandler,
		"OPTIONS /s5/upload/tus":     wrappedTusHandler,
		"HEAD /s5/upload/tus/:id":    tusHandler,
		"POST /s5/upload/tus/:id":    tusHandler,
		"PATCH /s5/upload/tus/:id":   tusHandler,
		"OPTIONS /s5/upload/tus/:id": wrappedTusHandler,

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

		"GET /swagger/swagger_spec": middleware.ApplyMiddlewares(byteHandler(jsonDoc)),
		"GET /swagger":              middleware.ApplyMiddlewares(swaggerHandler),
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

func BuildS5TusApi(identity ed25519.PrivateKey, accounts *account.AccountServiceDefault, storage *storage.StorageServiceDefault) jape.Handler {
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
	tusHandler := middleware.ApplyMiddlewares(tusJapeHandler, BuildTusCors(), middleware.AuthMiddleware(identity, accounts), injectJwt, protocolMiddleware, stripPrefix, middleware.ProxyMiddleware)

	return tusHandler
}
