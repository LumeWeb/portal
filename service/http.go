package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/samber/lo"
	"go.lumeweb.com/httputil"
	"go.lumeweb.com/portal-middleware/cors"
	"go.lumeweb.com/portal-middleware/swagger"
	"go.lumeweb.com/portal/build"
	otelecho "go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"sync"
	"time"

	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/web_manifest"
	ihttp "go.lumeweb.com/portal/service/internal/http"
	"go.uber.org/zap"
)

const (
	defaultManifestPath    = "mf-manifest.json"
	webBundleApiBasePath   = "/api/meta/plugin"
	webBundleSubPath       = "/%s/bundle/%s/"
	webBundleBasePath      = webBundleApiBasePath + webBundleSubPath
	webBundleManifestRoute = webBundleBasePath + defaultManifestPath

	// apiCatalogPath is the RFC 9727 well-known URI for API discovery. It is
	// served from the root of every hostname the portal publishes APIs on.
	apiCatalogPath = "/.well-known/api-catalog"
	// apiCatalogMediaType is the linkset JSON format used by RFC 9727 (RFC 9584).
	apiCatalogMediaType = "application/linkset+json"
	// openAPIMediaType is the OpenAPI 3.1 service-description media type.
	openAPIMediaType = "application/vnd.oai.openapi+json;version=3.1"
)

// apiCatalogLink is a single link relation in an api-catalog linkset entry.
type apiCatalogLink struct {
	Href string `json:"href"`
	Type string `json:"type"`
}

// apiCatalogEntry is one anchor (API base URI) and its link relations.
type apiCatalogEntry struct {
	Anchor      string           `json:"anchor"`
	ServiceDesc []apiCatalogLink `json:"service-desc"`
}

// apiCatalog is the RFC 9727 discovery document served at /.well-known/api-catalog.
type apiCatalog struct {
	Linkset []apiCatalogEntry `json:"linkset"`
}

var (
	pluginIDRegex    = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	genericTypeRegex = regexp.MustCompile(`^(.+?)\[(.+)]$`)
)

func jsonSchemaMapper(_ reflect.Type) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Title:                "JSON Data",
		Description:          "Arbitrary JSON data",
		AdditionalProperties: &jsonschema.Schema{},
	}
}

var defaultSchemaMappers = map[string]func(reflect.Type) *jsonschema.Schema{
	"JSON": jsonSchemaMapper,
}

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
	*core.BaseComponent
	router        router.Router
	srv           *http.Server
	access        core.AccessService
	bundleCache   sync.Map
	fsCache       sync.Map // Cache for bundle filesystems
	globalPaths   []string
	globalPathsMu sync.RWMutex
	apiCatalog    []byte // Precomputed RFC 9727 catalog; immutable after Init
	wg            sync.WaitGroup
	stopOnce      sync.Once
}

func NewHTTPService() (core.Service, []core.ContextBuilderOption, error) {
	_http := &HTTPServiceDefault{}

	protocols := &http.Protocols{}
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Protocols: protocols,
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_http.access = ctx.Service(core.ACCESS_SERVICE).(core.AccessService)

			_router, err := router.NewRouter(router.APIInfo().Title(fmt.Sprintf("%s Meta API", ctx.Config().Config().Core.PortalName)).Version(build.GetInfo().Version), func(c *router.RouterConfig) {
				c.Options.CustomServeHTTPHandler = _http
			}, router.WithReflectorOptions(&jsonschema.Reflector{
				Mapper: mapSchemaType,
				Namer: func(t reflect.Type) string {
					return nameGenerics(t)
				},
			}))
			if err != nil {
				return err
			}

			_http.router = _router
			srv.Handler = _http.router

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return _http.Stop()
		}),
	)

	_http.srv = srv

	return _http, opts, nil
}

func (h *HTTPServiceDefault) ID() string {
	return core.HTTP_SERVICE
}

func (h *HTTPServiceDefault) Router() router.Router {
	return h.router
}

func (h *HTTPServiceDefault) Init() error {
	// Register default global paths
	if err := h.RegisterGlobalPath("/api/meta"); err != nil {
		return fmt.Errorf("failed to register global path: %w", err)
	}
	if err := h.RegisterGlobalPath("/swagger"); err != nil {
		return fmt.Errorf("failed to register global path: %w", err)
	}
	if err := h.RegisterGlobalPath(apiCatalogPath); err != nil {
		return fmt.Errorf("failed to register global path: %w", err)
	}

	h.router.Use(echoMiddleware.RecoverWithConfig(echoMiddleware.RecoverConfig{
		StackSize: 1 << 10,
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			return err
		},
	}))

	ocfg := h.Config().Config().Core.Observability
	if ocfg.IsTracingEnabled() {
		h.router.Use(otelecho.Middleware(ocfg.ServiceName))
	}

	h.srv.Addr = ":" + strconv.FormatUint(uint64(h.Config().Config().Core.Port), 10)
	for _, api := range core.GetAPIs() {
		domain := h.getAPIDomain(api)

		// Create a gswagger router wrapping the mux subrouter
		apiInfo := api.OpenAPIInfo() // Get info from the API

		if apiInfo != nil {
			// If the API didn't explicitly set a version, use the plugin's build version
			if apiInfo.GetVersion() == "" {
				pluginID := api.Name()
				pluginInfo := core.GetPlugin(pluginID)
				if pluginInfo.Version != nil {
					apiInfo.Version(pluginInfo.Version.GetVersion())
				} else {
					// Fallback if plugin version is also not available
					apiInfo.Version("unknown")
				}
			}
		}

		// Create a subrouter for this API's domain
		hostRouter, err := h.Router().Host(domain)
		if err != nil {
			return fmt.Errorf("failed to create host router for API %s: %w", api.Name(), err)
		}

		if ocfg.IsTracingEnabled() {
			hostRouter.Use(otelecho.Middleware(ocfg.ServiceName))
		}

		if apiInfo != nil {
			router.UpdateRouterInfo(hostRouter, apiInfo)
		}

		// Configure the main API using the gswagger router
		err = api.Configure(hostRouter, h.access)
		if err != nil {
			return err
		}

		// Apply any registered extensions using the *same* gswagger router
		for _, ext := range core.GetAPIExtensions(api.Name()) {
			h.Logger().Info("Applying API extension",
				zap.String("api", api.Name()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			// The APIExtension.Configure method signature needs to change
			if err = ext.Configure(hostRouter, h.access); err != nil {
				return fmt.Errorf("failed to configure API extension: %w", err)
			}
		}

		corsCfg := core.CORSConfig{}

		if apicors, ok := api.(core.APICors); ok {
			corsCfg = apicors.CORSConfig()
		}

		router.GetRouter(hostRouter).OPTIONS("/api/*", func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		}, echo.WrapMiddleware(cors.NewWithDefaults(corsCfg)))

		if apiInfo != nil {
			// Generate and expose the OpenAPI spec for this API's router
			if err = hostRouter.GenerateAndExposeOpenapi(); err != nil {
				return fmt.Errorf("failed to generate openapi for API %s: %w", api.Name(), err)
			}
		}
	}

	// Build the RFC 9727 API catalog once. The set of APIs and plugins is fixed
	// after boot, so the catalog is computed a single time and served thereafter.
	if err := h.buildAPICatalog(); err != nil {
		return fmt.Errorf("failed to build api catalog: %w", err)
	}

	// Serve the RFC 9727 API catalog on every hostname. Registering it as a
	// global path makes the root router handle it regardless of the Host header.
	router.GetRouter(h.Router()).GET(apiCatalogPath, h.apiCatalogHandler,
		echo.WrapMiddleware(cors.NewWithDefaults(core.CORSConfig{})))

	rootApi, err := h.Router().Group("/api")
	if err != nil {
		return fmt.Errorf("failed to generate default api group: %w", err)
	}

	err = router.RegisterRoutes(rootApi, h.access, "", router.DefineRoutes(
		router.NewRoute(http.MethodGet, "/meta", h.apiMetaHandler,
			router.WithSwagger(
				router.WithSummary("Get Portal Metadata"),
				router.WithDescription("Returns metadata about installed plugins and their web bundles"),
				router.WithTags("Public"),
				router.WithQueryParam("app", "Filter metadata by application type", ""),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponses(
						router.DefineSwaggerErrorResponse(http.StatusInternalServerError, "Internal server error"),
					),
				),
			),
		),
	), router.WithCors())
	if err != nil {
		return err
	}

	pluginApi, err := rootApi.Group("/meta/plugin")

	err = router.RegisterRoutes(pluginApi, h.access, "", router.DefineRoutes(
		router.NewRoute(http.MethodGet, fmt.Sprintf(webBundleSubPath, ":plugin_id", ":bundle_id")+"*", h.apiPluginWebBundleFileServerHandler,
			router.WithSwagger(
				router.WithSummary("Get Plugin Web Bundle File"),
				router.WithDescription("Serves static files from a plugin's web bundle"),
				router.WithTags("Public"),
				router.WithPathParam("plugin_id", "Plugin identifier", "string"),
				router.WithPathParam("bundle_id", "Bundle index number", "integer"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponses(
						router.DefineSwaggerErrorResponse(http.StatusNotFound, "Plugin, bundle or file not found"),
						router.DefineSwaggerErrorResponse(http.StatusBadRequest, "Invalid bundle ID"),
						router.DefineSwaggerErrorResponse(http.StatusInternalServerError, "Failed to serve file"),
					),
				),
			),
		),
	), router.WithCors())
	if err != nil {
		return err
	}

	router.GetRouter(h.Router()).OPTIONS("/api/*", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, echo.WrapMiddleware(cors.NewWithDefaults(core.CORSConfig{})))

	err = swagger.WireRouter(h.router, "/swagger.json", "/swagger")
	if err != nil {
		return err
	}

	// Register metrics endpoints if observability is enabled
	if ocfg.IsMetricsEnabled() {
		metricsPath := ocfg.Metrics.Path
		if err := h.registerMetricsEndpoints(metricsPath, ocfg.Metrics); err != nil {
			return fmt.Errorf("failed to register metrics endpoints: %w", err)
		}
	}

	err = h.router.GenerateAndExposeOpenapi()
	if err != nil {
		return err
	}

	return nil
}

func (h *HTTPServiceDefault) apiMetaHandler(e echo.Context) error {
	ctx := httputil.Context(e)

	// Get app type from query param (empty string means no filter)
	appType := ctx.QueryParam("app")

	metaBuilder := NewPortalMetaBuilder(h.Config().Config().Core.Domain)

	// Add core build info from build.Default
	metaBuilder.AddCoreBuildInfo(build.Default.Info())

	// Process all plugins
	for _, plugin := range core.GetPlugins() {
		// Skip plugins that don't target the requested app type
		if appType != "" && !pluginTargetsApp(&plugin, appType) {
			continue
		}

		// Get plugin meta builder
		pluginBuilder, err := metaBuilder.AddPlugin(plugin.ID)
		if err != nil {
			h.Logger().Error("Failed to add plugin to meta builder",
				zap.String("plugin", plugin.ID),
				zap.Error(err))
			continue
		}

		// Add plugin build info if available
		if plugin.Version != nil {
			pluginBuilder.AddBuildInfo(plugin.Version.Info())
		}

		// Add web bundles
		for i, bundle := range plugin.WebBundles {
			if bundleTargetsApp(bundle, appType) {
				bundleURI := fmt.Sprintf(webBundleManifestRoute, plugin.ID, strconv.Itoa(i))
				pluginBuilder.AddWebBundle(bundleURI)
			}
		}

		// Let plugin add its own metadata
		if plugin.Meta != nil {
			if err := plugin.Meta(h.Context(), metaBuilder); err != nil {
				h.Logger().Error("Failed to process plugin meta",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
			}
		}
	}

	_ = ctx.Encode(metaBuilder.Build())

	return nil
}

// rootDomainURL returns the scheme-qualified URL for the portal's root domain,
// which hosts the core meta API and shares its hostname with APIs that define
// no subdomain of their own.
func (h *HTTPServiceDefault) rootDomainURL() string {
	protocol := "http"
	if h.Config().Config().Core.Secure {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s", protocol, h.Config().Config().Core.Domain)
}

// buildAPICatalog precomputes the RFC 9727 API discovery catalog into
// h.apiCatalog. It lists every published API (each hostname that exposes an
// OpenAPI spec) so autonomous clients can discover the portal's API surface
// without prior knowledge. The set of APIs and plugins is fixed after boot, so
// the catalog is built once during Init and served thereafter without
// recomputation.
func (h *HTTPServiceDefault) buildAPICatalog() error {
	var entries []apiCatalogEntry

	// Root domain entry: the core meta API and any APIs without their own subdomain.
	rootURL := h.rootDomainURL()
	entries = append(entries, apiCatalogEntry{
		Anchor: rootURL,
		ServiceDesc: []apiCatalogLink{
			{Href: rootURL + "/swagger.json", Type: openAPIMediaType},
		},
	})

	// One entry per API that generates and exposes its own OpenAPI spec.
	for _, api := range core.GetAPIList() {
		if api.OpenAPIInfo() == nil {
			continue // no spec generated for this API
		}
		if strings.TrimSpace(api.Subdomain()) == "" {
			continue // hosts on the root domain, already covered above
		}
		anchor := h.APISubdomain(api.Name(), true)
		if anchor == "" {
			continue
		}
		entries = append(entries, apiCatalogEntry{
			Anchor: anchor,
			ServiceDesc: []apiCatalogLink{
				{Href: anchor + "/swagger.json", Type: openAPIMediaType},
			},
		})
	}

	data, err := json.Marshal(apiCatalog{Linkset: entries})
	if err != nil {
		return err
	}
	h.apiCatalog = data
	return nil
}

// apiCatalogHandler serves the precomputed RFC 9727 discovery catalog at
// /.well-known/api-catalog. The same document is returned regardless of which
// hostname it is requested from.
func (h *HTTPServiceDefault) apiCatalogHandler(c echo.Context) error {
	return c.Blob(http.StatusOK, apiCatalogMediaType, h.apiCatalog)
}

func (h *HTTPServiceDefault) generateWebBundleURI(pluginID string, bundleIndex int) string {
	return fmt.Sprintf(webBundleBasePath, pluginID, strconv.Itoa(bundleIndex))
}

// getOrCreateBundleFilesystem atomically gets or creates a cached filesystem
func (h *HTTPServiceDefault) getOrCreateBundleFilesystem(pluginID string, bundleIndex int, bundle *core.WebBundle) *ihttp.BundleFileSystem {
	cacheKey := fmt.Sprintf("%s-%d", pluginID, bundleIndex)

	// Load or store the filesystem atomically
	actual, _ := h.fsCache.LoadOrStore(cacheKey, ihttp.NewBundleFileSystem(bundle, bundle.FSPrefix))
	return actual.(*ihttp.BundleFileSystem)
}

func (h *HTTPServiceDefault) getWebBundleManifestName(pluginID string, bundleIndex int) string {
	plugin := core.GetPlugin(pluginID)

	if bundleIndex < len(plugin.WebBundles) {
		bundle := plugin.WebBundles[bundleIndex]
		if bundle != nil {
			if bundle.ManifestPath != "" {
				return bundle.ManifestPath
			}
		}
	}

	return defaultManifestPath
}

func (h *HTTPServiceDefault) getProcessedManifest(plugin *core.PluginInfo, bundle *core.WebBundle, index int) (*web_manifest.Manifest, error) {
	// Get or create cached filesystem
	fs := h.getOrCreateBundleFilesystem(plugin.ID, index, bundle)

	file, err := fs.Open(lo.CoalesceOrEmpty(bundle.ManifestPath, defaultManifestPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	defer file.Close()

	manifestData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest web_manifest.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Update public path
	baseURL := fmt.Sprintf(webBundleBasePath, plugin.ID, strconv.Itoa(index))
	manifest.MetaData.PublicPath = baseURL

	return &manifest, nil
}

func (h *HTTPServiceDefault) apiPluginWebBundleFileServerHandler(e echo.Context) error {
	ctx := httputil.Context(e)

	pluginId := ctx.Param("plugin_id")
	bundleId := ctx.Param("bundle_id")

	// Validate plugin ID (alphanumeric and hyphens only)
	if !pluginIDRegex.MatchString(pluginId) {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid plugin ID format")
	}

	// Get plugin
	plugin := core.GetPlugin(pluginId)
	if plugin.ID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "Plugin not found")
	}

	// Parse and validate bundle ID
	bundleIndex, err := strconv.Atoi(bundleId)
	if err != nil || bundleIndex >= len(plugin.WebBundles) {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid bundle ID")
	}

	bundle := plugin.WebBundles[bundleIndex]
	if bundle == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Bundle not found")
	}

	fs := h.getOrCreateBundleFilesystem(pluginId, bundleIndex, bundle)

	// Strip the prefix from the request path
	prefix := h.generateWebBundleURI(pluginId, bundleIndex)
	baseFileServer := http.FileServer(fs)

	// Wrap the file server to handle manifest processing
	fileServer := http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle manifest requests with processed version
		if path.Base(r.URL.Path) == h.getWebBundleManifestName(pluginId, bundleIndex) {
			manifest, err := h.getProcessedManifest(&plugin, bundle, bundleIndex)
			if err != nil {
				http.Error(w, "Failed to process manifest", http.StatusInternalServerError)
				h.Logger().Error("Failed to process manifest", zap.Error(err))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(manifest)
			if err != nil {
				http.Error(w, "Failed to encode manifest", http.StatusInternalServerError)
				h.Logger().Error("Failed to encode manifest", zap.Error(err))
			}
			return
		}

		baseFileServer.ServeHTTP(w, r)
	}))

	// Add cache headers based on file type
	// Manifests might need shorter cache times for updates
	if path.Ext(e.Request().URL.Path) == ".json" {
		e.Response().Header().Set("Cache-Control", "public, max-age=3600") // 1 hour for JSON files
	} else {
		e.Response().Header().Set("Cache-Control", "public, max-age=31536000") // 1 year for static assets
	}

	// Serve the file
	fileServer.ServeHTTP(e.Response(), e.Request())

	return nil
}

func (h *HTTPServiceDefault) Serve() error {
	ln, err := net.Listen("tcp", h.srv.Addr)
	if err != nil {
		return err
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		err := h.srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			h.Logger().Fatal("Failed to serve", zap.Error(err))
		}
	}()

	h.wg.Wait()
	return nil
}

func (h *HTTPServiceDefault) Stop() error {
	var err error
	h.stopOnce.Do(func() {
		if h.srv == nil {
			return // Already stopped or never started
		}

		// Use fresh context with timeout for shutdown
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = h.srv.Shutdown(timeoutCtx)
		h.wg.Wait() // Wait for server goroutine to complete
	})
	return err
}

func (h *HTTPServiceDefault) Port() uint16 {
	portStr := strings.TrimPrefix(h.srv.Addr, ":")
	port, _ := strconv.ParseUint(portStr, 10, 16)
	return uint16(port)
}

func (h *HTTPServiceDefault) RegisterGlobalPath(path string) error {
	h.globalPathsMu.Lock()
	defer h.globalPathsMu.Unlock()

	// Validate path
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /")
	}

	// Check if already exists
	if lo.Contains(h.globalPaths, path) {
		return nil // Already registered
	}

	h.globalPaths = append(h.globalPaths, path)
	return nil
}

func (h *HTTPServiceDefault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This method is set as the CustomServeHTTPHandler on the root gswagger.Router.
	// It is called after gswagger's standard documentation path checks.

	// Prioritize global routes, which are always served by the root router
	isGlobalPath := false
	h.globalPathsMu.RLock()
	for _, globalPath := range h.globalPaths {
		if strings.HasPrefix(r.URL.Path, globalPath) {
			isGlobalPath = true
			break
		}
	}
	h.globalPathsMu.RUnlock()
	if isGlobalPath {
		// Attempt to cast the Root gswagger Router's underlying framework router to an http.Handler
		if handler, ok := h.router.Router().Router(true).(http.Handler); ok {
			handler.ServeHTTP(w, r)
			return // Request handled by the root framework router
		}
		// This should not happen if the router is properly configured
		h.Logger().Error("Root router does not implement http.Handler")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If not /api/meta, determine the target gswagger Router based on the Host header
	var targetRouter router.Router
	host := h.normalizeHost(r.Host)

	// Use GetHostRouter to find a host-specific router
	hostRouter := h.router.GetHostRouter(host)

	if hostRouter != nil {
		// If a Host Router is found, the target is the found Host Router.
		targetRouter = hostRouter
	} else {
		// If no Host Router is found, the target is the Root gswagger Router
		targetRouter = h.router.GetRootRouter()
	}

	// Attempt to cast the Target gswagger Router's underlying framework router to an http.Handler
	if handler, ok := targetRouter.Router().Router(true).(http.Handler); ok {
		handler.ServeHTTP(w, r)
		return
	}

	// This should not happen if the router is properly configured
	h.Logger().Error("Target router does not implement http.Handler", zap.String("host", host))
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (h *HTTPServiceDefault) getAPIDomain(api core.API) string {
	root := strings.Trim(strings.ToLower(h.Config().Config().Core.Domain), ".")
	sub := strings.Trim(strings.ToLower(strings.TrimSpace(api.Subdomain())), ".")
	host := root
	if sub != "" {
		host = sub + "." + root
	}
	return net.JoinHostPort(host, strconv.FormatUint(uint64(h.getActivePort()), 10))
}

func (h *HTTPServiceDefault) getActivePort() uint16 {
	var port uint
	port = h.Config().Config().Core.Port
	if h.Config().Config().Core.ExternalPort != 0 {
		port = h.Config().Config().Core.ExternalPort
	}
	return uint16(port)
}

func (h *HTTPServiceDefault) normalizeHost(host string) string {
	// Handle empty host
	if host == "" {
		return host
	}

	// Split host and port
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		// If no port in host, add our active port
		if strings.Contains(err.Error(), "missing port") {
			return net.JoinHostPort(host, strconv.FormatUint(uint64(h.getActivePort()), 10))
		}
		return host
	}

	// Always return the original host:port if SplitHostPort succeeded
	return net.JoinHostPort(hostname, port)
}

func (h *HTTPServiceDefault) APISubdomain(id string, proto bool) string {
	formatter := ""

	protocol := "http"
	if h.Config().Config().Core.Secure {
		protocol = "https"
	}

	if proto {
		formatter += fmt.Sprintf("%s://", protocol)
	}

	formatter += "%s.%s"

	if core.GetAPI(id) == nil {
		return ""
	}

	return fmt.Sprintf(formatter, core.GetAPI(id).Subdomain(), h.Config().Config().Core.Domain)
}

func mapSchemaType(typ reflect.Type) *jsonschema.Schema {
	// First try our default mappers
	if mapper, exists := defaultSchemaMappers[typ.Name()]; exists {
		return mapper(typ)
	}

	// Then check APIs that implement APISchemer
	for _, api := range core.GetAPIs() {
		if schemer, ok := api.(core.APISchemer); ok {
			if schema := schemer.GetSchemaType()(typ); schema != nil {
				return schema
			}
		}
	}

	return nil
}

// Helper function to check if plugin targets specified app (or is universal)
func pluginTargetsApp(plugin *core.PluginInfo, appType string) bool {
	// Plugins with no TargetApps work with all applications
	if len(plugin.TargetApps) == 0 {
		return true
	}

	// Check if plugin targets this app or is core
	for _, target := range plugin.TargetApps {
		if target == appType || target == "core" {
			return true
		}
	}
	return false
}

// Helper function to check if plugin targets specified app (or is universal)
func bundleTargetsApp(bundle *core.WebBundle, appType string) bool {
	// Bundle with no TargetApps work with all applications
	if len(bundle.TargetApps) == 0 {
		return true
	}

	// Check if plugin targets this app or is core
	for _, target := range bundle.TargetApps {
		if target == appType {
			return true
		}
	}
	return false
}

func getTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem() // Dereference the pointer
	}

	pkgPath := t.PkgPath()
	name := t.Name()

	if pkgPath != "" && name != "" {
		parts := strings.Split(pkgPath, "/")
		pkgName := parts[len(parts)-1]             // Last part of the path
		return fmt.Sprintf("%s.%s", pkgName, name) // Shortened package + name
	}
	if name != "" {
		return name
	}
	return "" // For built-in types
}

// getGenericName converts generic type names to a more readable format.
// Handles two cases:
// 1. Generic types in format "Parent[Child]" → returns "ChildParent"
// 2. Pointer types "*Type" → returns "Type" (only removes leading asterisk)
func getGenericName(name string) string {
	if matches := genericTypeRegex.FindStringSubmatch(name); matches != nil {
		parent := matches[1]
		child := matches[2]
		return fmt.Sprintf("%sOf%s", parent, child) // "Of" separator
	}
	return strings.TrimPrefix(name, "*") // Remove only leading pointer prefix
}

// nameGenerics generates consistent names for generic types in OpenAPI schemas.
// Handles:
// - Pointer types by dereferencing (*Type → Type)
// - Generic types by combining parent/child names (Parent[Child] → ChildParent)
// - Error types by returning their direct name
// - Package-qualified names by stripping package path (pkg.Type → Type)
func nameGenerics(r reflect.Type) string {
	// Dereference pointer types
	if r.Kind() == reflect.Ptr {
		r = r.Elem()
	}

	name := getTypeName(r)

	// Only process struct types - return simplified name for others
	if r.Kind() != reflect.Struct {
		return stripPackageName(getGenericName(name))
	}

	// Special handling for error types - use their direct name
	if r.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return r.Name()
	}

	// Handle generic types with parameters
	if matches := genericTypeRegex.FindStringSubmatch(name); matches != nil {
		typeParam := matches[2]
		if typeParam != "" {
			parts := strings.Split(typeParam, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				modelParts := strings.Split(lastPart, ".")
				if len(modelParts) > 0 {
					baseTypeName := modelParts[len(modelParts)-1]
					parentName := matches[1]
					parentParts := strings.Split(parentName, ".")
					parentSimpleName := ""
					if len(parentParts) > 0 {
						parentSimpleName = parentParts[len(parentParts)-1]
					}
					finalName := baseTypeName + parentSimpleName
					return finalName
				}
			}
		}
	}

	// Default case - strip package name and apply generic name formatting
	return stripPackageName(getGenericName(name))
}

func stripPackageName(name string) string {
	parts := strings.Split(name, ".")
	return parts[len(parts)-1]
}

// registerMetricsEndpoints registers the core metrics endpoint and per-vhost metrics endpoints
func (h *HTTPServiceDefault) registerMetricsEndpoints(metricsPath string, metricsCfg config.MetricsConfig) error {
	metricsAuth := metricsBasicAuthMiddleware(metricsCfg)

	coreMiddleware := []echo.MiddlewareFunc{
		echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
			Registerer: core.CoreMetricsRegistry(),
		}),
	}
	coreMiddleware = append(coreMiddleware, metricsAuth...)

	router.GetRouter(h.Router()).GET(metricsPath, echoprometheus.NewHandlerWithConfig(echoprometheus.HandlerConfig{Gatherer: core.CoreMetricsRegistry()}), coreMiddleware...)

	h.Logger().Info("Registered core metrics endpoint", zap.String("path", metricsPath))

	for _, api := range core.GetAPIs() {
		apiName := api.Name()
		domain := h.getAPIDomain(api)
		hostRouter := h.router.GetHostRouter(h.normalizeHost(domain))
		if hostRouter == nil {
			h.Logger().Warn("Failed to get host router for metrics",
				zap.String("api", apiName))
			continue
		}

		apiMiddleware := []echo.MiddlewareFunc{
			echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
				Subsystem:  api.ID(),
				Registerer: core.PluginMetricsRegistry(apiName),
			}),
		}
		apiMiddleware = append(apiMiddleware, metricsAuth...)

		router.GetRouter(hostRouter).GET(metricsPath, echoprometheus.NewHandlerWithConfig(echoprometheus.HandlerConfig{Gatherer: core.PluginMetricsRegistry(apiName)}), apiMiddleware...)

		h.Logger().Info("Registered API metrics endpoint",
			zap.String("api", apiName),
			zap.String("domain", domain),
			zap.String("path", metricsPath))
	}

	return nil
}

func metricsBasicAuthMiddleware(metricsCfg config.MetricsConfig) []echo.MiddlewareFunc {
	if !metricsCfg.BasicAuth.IsEnabled() {
		return nil
	}
	return []echo.MiddlewareFunc{
		echoMiddleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
			return subtle.ConstantTimeCompare([]byte(password), []byte(metricsCfg.BasicAuth.Password)) == 1, nil
		}),
	}
}
