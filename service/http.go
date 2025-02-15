package service

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"go.lumeweb.com/httputil"
	"go.lumeweb.com/portal-middleware/auth/jwt"
	"go.lumeweb.com/portal-middleware/cors"
	"go.lumeweb.com/portal-middleware/middleware"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/web_manifest"
	ihttp "go.lumeweb.com/portal/service/internal/http"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"path"
	"strconv"
	"sync"
)

const (
	defaultManifestPath    = "mf-manifest.json"
	webBundleBasePath      = "/api/meta/plugin/%s/bundle/%d/"
	webBundleManifestRoute = webBundleBasePath + defaultManifestPath
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
	router      *mux.Router
	srv         *http.Server
	access      core.AccessService
	bundleCache sync.Map
	fsCache     sync.Map // Cache for bundle filesystems
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

func (h *HTTPServiceDefault) Router() *mux.Router {
	return h.router
}

func (h *HTTPServiceDefault) Init() error {
	h.router.Use(handlers.RecoveryHandler(handlers.RecoveryLogger(&recoverLogger{h.ctx})))
	h.srv.Addr = ":" + strconv.FormatUint(uint64(h.ctx.Config().Config().Core.Port), 10)
	for _, api := range core.GetAPIs() {
		subdomain := api.Subdomain()
		domain := fmt.Sprintf("%s.%s", api.Subdomain(), h.ctx.Config().Config().Core.Domain)

		if subdomain == "" {
			domain = h.ctx.Config().Config().Core.Domain
		}

		// Create a mux subrouter for this API's domain
		muxRouter := h.Router().Host(domain).Subrouter()

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
		router, err := httputil.NewSwaggerRouter(muxRouter, apiInfo)
		if err != nil {
			return fmt.Errorf("failed to create swagger router for API %s: %w", api.Name(), err)
		}

		// Configure the main API using the gswagger router
		// The API.Configure method signature needs to change
		err = api.Configure(router, h.access)
		if err != nil {
			return err
		}

		// Apply any registered extensions using the *same* gswagger router
		for _, ext := range core.GetAPIExtensions(api.Name()) {
			h.logger.Info("Applying API extension",
				zap.String("api", api.Name()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			// The APIExtension.Configure method signature needs to change
			if err := ext.Configure(router, h.access); err != nil {
				return fmt.Errorf("failed to configure API extension: %w", err)
			}
		}

		// Generate and expose the OpenAPI spec for this API's router
		if err := router.GenerateAndExposeOpenapi(); err != nil {
			return fmt.Errorf("failed to generate openapi for API %s: %w", api.Name(), err)
		}
	}

	authMw := middleware.AuthMiddleware(h.ctx, jwt.PurposeLogin)

	h.Router().PathPrefix("/debug/").Handler(http.DefaultServeMux).Use(authMw)

	corsHandler := cors.New(cors.Config{})

	rootApi := h.Router().PathPrefix("/api").Subrouter()
	rootApi.Use(corsHandler)
	rootApi.HandleFunc("/meta", h.apiMetaHandler).Methods(http.MethodGet)
	pluginRouter := rootApi.PathPrefix("/meta/plugin").Subrouter()

	pluginRouter.HandleFunc(fmt.Sprintf(webBundleManifestRoute, "{plugin_id}", "{bundle_id}"), h.apiPluginWebBundleFileServerHandler).Methods(http.MethodGet)
	pluginRouter.PathPrefix("/{plugin_id}/bundle/{bundle_id}/").HandlerFunc(h.apiPluginWebBundleFileServerHandler)

	return nil
}

func (h *HTTPServiceDefault) apiMetaHandler(w http.ResponseWriter, r *http.Request) {
	ctx := httputil.Context(r, w)

	// Get app type from query param (empty string means no filter)
	appType := r.URL.Query().Get("app")

	metaBuilder := NewPortalMetaBuilder(h.ctx.Config().Config().Core.Domain)

	// First pass: Add plugin IDs with version info
	for _, plugin := range core.GetPlugins() {
		// Skip plugins only if we're filtering by app AND plugin doesn't target this app
		if appType != "" && !pluginTargetsApp(&plugin, appType) {
			continue
		}

		metaBuilder.AddPlugin(plugin.ID)
		if plugin.Version != nil {
			metaBuilder.AddPluginBuildInfo(plugin.ID, plugin.Version.Info())
		}
	}

	// Second pass: Process plugin meta and web bundles
	for _, plugin := range core.GetPlugins() {
		// Same filtering logic as above
		if appType != "" && !pluginTargetsApp(&plugin, appType) {
			continue
		}

		// Add web bundle URLs using processed manifests
		for i, bundle := range plugin.WebBundles {
			bundleURI := fmt.Sprintf(webBundleManifestRoute, plugin.ID, i)
			if bundleTargetsApp(bundle, appType) {
				metaBuilder.AddPluginWebBundle(plugin.ID, bundleURI)
			}
		}

		// Process plugin-specific meta
		if plugin.Meta != nil {
			if err := plugin.Meta(h.ctx, metaBuilder); err != nil {
				http.Error(w, "Failed to build meta", http.StatusInternalServerError)
				h.logger.Error("Failed to build meta", zap.Error(err))
				return
			}
		}
	}

	ctx.Encode(metaBuilder.Build())
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

	if len(plugin.WebBundles) <= bundleIndex {
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

func (h *HTTPServiceDefault) apiPluginWebBundleFileServerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pluginId := vars["plugin_id"]
	bundleId := vars["bundle_id"]

	// Get plugin
	plugin := core.GetPlugin(pluginId)
	if plugin.ID == "" {
		http.Error(w, "Plugin not found", http.StatusNotFound)
		return
	}

	// Parse and validate bundle ID
	bundleIndex, err := strconv.Atoi(bundleId)
	if err != nil || bundleIndex >= len(plugin.WebBundles) {
		http.Error(w, "Invalid bundle ID", http.StatusBadRequest)
		return
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

	// Add cache headers
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	// Serve the file
	fileServer.ServeHTTP(w, r)
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
