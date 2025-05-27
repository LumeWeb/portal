package service

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/samber/lo"
	"go.lumeweb.com/httputil"
	"go.lumeweb.com/portal-middleware/auth/jwt"
	"go.lumeweb.com/portal-middleware/cors"
	"go.lumeweb.com/portal-middleware/middleware"
	"go.lumeweb.com/portal/build"
	"regexp"

	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/web_manifest"
	ihttp "go.lumeweb.com/portal/service/internal/http"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"sync"
)

const (
	defaultManifestPath    = "mf-manifest.json"
	webBundleBasePath      = "/api/meta/plugin/%s/bundle/%d/"
	webBundleManifestRoute = webBundleBasePath + defaultManifestPath
)

var (
	pluginIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
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
	ctx         core.Context
	logger      *core.Logger
	router      router.Router
	srv         *http.Server
	access      core.AccessService
	bundleCache sync.Map
	fsCache     sync.Map // Cache for bundle filesystems
}

//var _ handlers.RecoveryHandlerLogger = (*recoverLogger)(nil)
/*
type recoverLogger struct {
	ctx core.Context
}

func (r *recoverLogger) Println(v ...interface{}) {
	r.ctx.Logger().Error("Recovered from panic", zap.Any("panic", v))
}*/

func NewHTTPService() (*HTTPServiceDefault, []core.ContextBuilderOption, error) {
	_router, err := router.NewRouter(router.APIInfo())
	if err != nil {
		return nil, nil, err
	}

	_http := &HTTPServiceDefault{
		router: _router,
	}

	srv := &http.Server{
		Handler: _http.router,
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_http.ctx = ctx
			_http.logger = ctx.ServiceLogger(_http)
			_http.access = ctx.Service(core.ACCESS_SERVICE).(core.AccessService)
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

func (h *HTTPServiceDefault) Router() router.Router {
	return h.router
}

func (h *HTTPServiceDefault) Init() error {

	h.router.Use(echoMiddleware.RecoverWithConfig(echoMiddleware.RecoverConfig{
		StackSize: 1 << 10,
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			return err
		},
	}))
	h.srv.Addr = ":" + strconv.FormatUint(uint64(h.ctx.Config().Config().Core.Port), 10)
	for _, api := range core.GetAPIs() {
		subdomain := api.Subdomain()
		domain := fmt.Sprintf("%s.%s", api.Subdomain(), h.ctx.Config().Config().Core.Domain)

		if subdomain == "" {
			domain = h.ctx.Config().Config().Core.Domain
		}

		// Create a gswagger router wrapping the mux subrouter
		apiInfo := api.OpenAPIInfo() // Get info from the API

		// If the API didn't explicitly set a version, use the plugin's build version
		if apiInfo.GetVersion() == "" {
			// Assuming you can get the plugin info associated with this API
			// You might need to adjust how plugins and APIs are linked or pass plugin info
			// to the API.Configure method or the OpenAPIInfo method.
			// For now, let's assume you can get the plugin ID.
			pluginID := api.Name()                 // Or some other way to get the plugin ID
			pluginInfo := core.GetPlugin(pluginID) // Assuming GetPlugin exists and works by API name or similar
			if pluginInfo.Version != nil {
				apiInfo.Version(pluginInfo.Version.GetVersion())
			} else {
				// Fallback if plugin version is also not available
				apiInfo.Version("unknown")
			}
		}

		// Create a subrouter for this API's domain
		hostRouter, err := h.Router().Host(domain)
		if err != nil {
			return fmt.Errorf("failed to create host router for API %s: %w", api.Name(), err)
		}

		// Configure the main API using the gswagger router
		err = api.Configure(hostRouter, h.access)
		if err != nil {
			return err
		}

		// Apply any registered extensions using the *same* gswagger router
		for _, ext := range core.GetAPIExtensions(api.Name()) {
			h.logger.Info("Applying API extension",
				zap.String("api", api.Name()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			// The APIExtension.Configure method signature needs to change
			if err = ext.Configure(hostRouter, h.access); err != nil {
				return fmt.Errorf("failed to configure API extension: %w", err)
			}
		}

		// Generate and expose the OpenAPI spec for this API's router
		if err = hostRouter.GenerateAndExposeOpenapi(); err != nil {
			return fmt.Errorf("failed to generate openapi for API %s: %w", api.Name(), err)
		}
	}

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
			),
			router.WithCustomErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusInternalServerError, "Internal server error"),
			),
		),
	), middleware.AuthMiddleware(h.ctx, jwt.PurposeLogin), echo.WrapMiddleware(cors.NewWithDefaults(cors.Config{})))
	if err != nil {
		return err
	}

	pluginApi, err := h.Router().Group("/meta/plugin")

	err = router.RegisterRoutes(pluginApi, h.access, "", router.DefineRoutes(
		router.NewRoute(http.MethodGet, fmt.Sprintf(webBundleManifestRoute, "{plugin_id}", "{bundle_id}"), h.apiMetaHandler,
			router.WithSwagger(
				router.WithSummary("Get Plugin Web Bundle Manifest"),
				router.WithDescription("Returns the processed web manifest for a plugin's web bundle"),
				router.WithTags("Public"),
				router.WithPathParam("plugin_id", "Plugin identifier", "string"),
				router.WithPathParam("bundle_id", "Bundle index number", "integer"),
			),
			router.WithCustomErrorResponses(
				router.DefineSwaggerErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Plugin or bundle not found"),
					router.DefineSwaggerErrorResponse(http.StatusBadRequest, "Invalid bundle ID"),
					router.DefineSwaggerErrorResponse(http.StatusInternalServerError, "Failed to process manifest"),
				),
			),
		),
		router.NewRoute(http.MethodGet, fmt.Sprintf(webBundleBasePath, "{plugin_id}", "{bundle_id}")+"*", h.apiPluginWebBundleFileServerHandler,
			router.WithSwagger(
				router.WithSummary("Get Plugin Web Bundle File"),
				router.WithDescription("Serves static files from a plugin's web bundle"),
				router.WithTags("Public"),
				router.WithPathParam("plugin_id", "Plugin identifier", "string"),
				router.WithPathParam("bundle_id", "Bundle index number", "integer"),
			),
			router.WithCustomErrorResponses(
				router.DefineSwaggerErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Plugin, bundle or file not found"),
					router.DefineSwaggerErrorResponse(http.StatusBadRequest, "Invalid bundle ID"),
					router.DefineSwaggerErrorResponse(http.StatusInternalServerError, "Failed to serve file"),
				),
			),
		),
	), middleware.AuthMiddleware(h.ctx, jwt.PurposeLogin), echo.WrapMiddleware(cors.NewWithDefaults(cors.Config{})))
	if err != nil {
		return err
	}

	return nil
}

func (h *HTTPServiceDefault) apiMetaHandler(e echo.Context) error {
	ctx := httputil.Context(e)

	// Get app type from query param (empty string means no filter)
	appType := ctx.QueryParam("app")

	metaBuilder := NewPortalMetaBuilder(h.ctx.Config().Config().Core.Domain)

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
			h.logger.Error("Failed to add plugin to meta builder",
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
				bundleURI := fmt.Sprintf(webBundleManifestRoute, plugin.ID, i)
				pluginBuilder.AddWebBundle(bundleURI)
			}
		}

		// Let plugin add its own metadata
		if plugin.Meta != nil {
			if err := plugin.Meta(h.ctx, metaBuilder); err != nil {
				h.logger.Error("Failed to process plugin meta",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
			}
		}
	}

	ctx.Encode(metaBuilder.Build())

	return nil
}

func (h *HTTPServiceDefault) generateWebBundleURI(pluginID string, bundleIndex int) string {
	return fmt.Sprintf(webBundleBasePath, pluginID, bundleIndex)
}

// getCachedManifest retrieves a cached manifest if it exists
func (h *HTTPServiceDefault) getCachedManifest(key string) (*web_manifest.Manifest, bool) {
	cached, ok := h.bundleCache.Load(key)
	if !ok {
		return nil, false
	}
	return cached.(*web_manifest.Manifest), true
}

// storeCachedManifest stores a manifest in the cache
func (h *HTTPServiceDefault) storeCachedManifest(key string, manifest *web_manifest.Manifest) {
	h.bundleCache.Store(key, manifest)
}

// getCachedFilesystem retrieves a cached filesystem if it exists
func (h *HTTPServiceDefault) getCachedFilesystem(key string) (*ihttp.BundleFileSystem, bool) {
	cached, ok := h.fsCache.Load(key)
	if !ok {
		return nil, false
	}
	return cached.(*ihttp.BundleFileSystem), true
}

// storeCachedFilesystem stores a filesystem in the cache
func (h *HTTPServiceDefault) storeCachedFilesystem(key string, fs *ihttp.BundleFileSystem) {
	h.fsCache.Store(key, fs)
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
	// Check cache first
	cacheKey := fmt.Sprintf("%s-%d", plugin.ID, index)
	if manifest, ok := h.getCachedManifest(cacheKey); ok {
		return manifest, nil
	}

	// Get or create cached filesystem
	fs, ok := h.getCachedFilesystem(cacheKey)
	if !ok {
		fs = ihttp.NewBundleFileSystem(bundle, bundle.FSPrefix)
		h.storeCachedFilesystem(cacheKey, fs)
	}

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
	baseURL := fmt.Sprintf(webBundleBasePath, plugin.ID, index)
	manifest.MetaData.PublicPath = baseURL

	// Cache the processed manifest
	h.storeCachedManifest(cacheKey, &manifest)

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

	// Get or create cached filesystem
	cacheKey := fmt.Sprintf("%s-%d", pluginId, bundleIndex)
	fs, ok := h.getCachedFilesystem(cacheKey)
	if !ok {
		fs = ihttp.NewBundleFileSystem(bundle, bundle.FSPrefix)
		h.storeCachedFilesystem(cacheKey, fs)
	}

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
				h.logger.Error("Failed to process manifest", zap.Error(err))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(manifest)
			if err != nil {
				http.Error(w, "Failed to encode manifest", http.StatusInternalServerError)
				h.logger.Error("Failed to encode manifest", zap.Error(err))
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

func (h *HTTPServiceDefault) APISubdomain(id string, proto bool) string {
	formatter := ""

	if proto {
		formatter += "https://"
	}

	formatter += "%s.%s"

	if core.GetAPI(id) == nil {
		return ""
	}

	return fmt.Sprintf(formatter, core.GetAPI(id).Subdomain(), h.ctx.Config().Config().Core.Domain)
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
