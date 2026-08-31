package service_tests

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/internal/service_tests/http/testdata/embed_bundle"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/web_manifest"
	"go.lumeweb.com/portal/service"
)

//go:embed http/testdata/web_bundle/*
var testWebBundleFS embed.FS

func TestHTTPService_apiMetaHandler_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// Test basic meta endpoint
		req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
		rec := httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)

		// Assertions
		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.NotEmpty(tb, rec.Body.String())
		assert.Equal(tb, "application/json", rec.Header().Get("Content-Type"))

		// Test with app query parameter
		core.RegisterPlugin(core.PluginInfo{
			ID:         "testplugin",
			TargetApps: []string{"testapp"},
			WebBundles: []*core.WebBundle{
				core.NewWebBundle(
					testWebBundleFS,
					core.WithWebBundlePrefix("http/testdata/web_bundle"),
				),
			},
		})

		req = httptest.NewRequest(http.MethodGet, "/api/meta?app=testapp", nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)

		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.NotEmpty(tb, rec.Body.String())
	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService))
}

func TestHTTPService_apiPluginWebBundleFileServerHandler_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// 1. Create a test plugin with a web bundle
		pluginID := "testplugin"
		bundleContent := "test bundle content"
		pluginInfo := core.PluginInfo{
			ID: pluginID,
			WebBundles: []*core.WebBundle{
				core.NewWebBundle(
					testWebBundleFS,
					core.WithWebBundlePrefix("http/testdata/web_bundle"),
				),
			},
		}
		core.RegisterPlugin(pluginInfo)

		// 2. Create a test request
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/test.html", pluginID), nil)
		rec := httptest.NewRecorder()

		// 3. Serve the request through the router
		httpService.Router().ServeHTTP(rec, req)

		// 4. Assertions
		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.Equal(tb, bundleContent, rec.Body.String())
		assert.Equal(tb, "public, max-age=31536000", rec.Header().Get("Cache-Control"))

		// Test with non-existent plugin
		req = httptest.NewRequest(http.MethodGet, "/api/meta/plugin/nonexistent/bundle/0/test.html", nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

		// Test with invalid bundle ID
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/invalid/index.html", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusBadRequest, rec.Code)

		// Test with non-existent file
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/missing.html", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService), coreTesting.WithAPIID(""))
}

func TestHTTPService_apiPluginWebBundleFileServerHandler_ServeManifest_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// 1. Create a test plugin with a web bundle
		pluginID := "testplugin"
		var manifest web_manifest.Manifest
		manifest.MetaData.PublicPath = "/api/meta/plugin/testplugin/bundle/0/"
		bundleContentBytes, err := json.Marshal(&manifest)
		require.NoError(t, err)
		bundleContent := string(bundleContentBytes)

		pluginInfo := core.PluginInfo{
			ID: pluginID,
			WebBundles: []*core.WebBundle{
				core.NewWebBundle(
					testWebBundleFS,
					core.WithWebBundlePrefix("http/testdata/web_bundle"),
				),
			},
		}
		core.RegisterPlugin(pluginInfo)

		// 2. Create a test request
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/mf-manifest.json", pluginID), nil)
		rec := httptest.NewRecorder()

		// 3. Serve the request through the router
		httpService.Router().ServeHTTP(rec, req)

		// 4. Assertions
		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.Equal(tb, bundleContent, strings.Trim(rec.Body.String(), "\n"))
		assert.Equal(tb, "public, max-age=3600", rec.Header().Get("Cache-Control"))

		// Test with non-existent plugin
		req = httptest.NewRequest(http.MethodGet, "/api/meta/plugin/nonexistent/bundle/0/mf-manifest.json", nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

		// Test with invalid bundle ID
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/invalid/mf-manifest.json", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusBadRequest, rec.Code)

		// Test with non-existent file
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/mf-manifest1.json", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService), coreTesting.WithAPIID(""))
}

func TestHTTPService_apiPluginWebBundleFileServerHandler_ServeManifest_EmbedBundle_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// 1. Create a test plugin with a web bundle
		pluginID := "testplugin"
		var manifest web_manifest.Manifest
		manifest.MetaData.PublicPath = "/api/meta/plugin/testplugin/bundle/0/"
		bundleContentBytes, err := json.Marshal(&manifest)
		require.NoError(t, err)
		bundleContent := string(bundleContentBytes)
		pluginInfo := core.PluginInfo{
			ID: pluginID,
			WebBundles: []*core.WebBundle{
				core.NewWebBundle(
					embed_bundle.GetFS(),
				),
			},
		}
		core.RegisterPlugin(pluginInfo)

		// 2. Create a test request
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/mf-manifest.json", pluginID), nil)
		rec := httptest.NewRecorder()

		// 3. Serve the request through the router
		httpService.Router().ServeHTTP(rec, req)

		// 4. Assertions
		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.Equal(tb, bundleContent, strings.Trim(rec.Body.String(), "\n"))
		assert.Equal(tb, "public, max-age=3600", rec.Header().Get("Cache-Control"))

		// Test with non-existent plugin
		req = httptest.NewRequest(http.MethodGet, "/api/meta/plugin/nonexistent/bundle/0/mf-manifest.json", nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

		// Test with invalid bundle ID
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/invalid/mf-manifest.json", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusBadRequest, rec.Code)

		// Test with non-existent file
		req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/meta/plugin/%s/bundle/0/mf-manifest1.json", pluginID), nil)
		rec = httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)
		assert.Equal(tb, http.StatusNotFound, rec.Code)

	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService), coreTesting.WithAPIID(""))
}

func TestHTTPService_ServeHTTP_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// Create a test request and recorder
		req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
		rec := httptest.NewRecorder()

		// Call the ServeHTTP method
		httpService.(*service.HTTPServiceDefault).ServeHTTP(rec, req)

		// Assertions
		assert.Equal(tb, http.StatusOK, rec.Code)
		assert.NotEmpty(tb, rec.Body.String())
	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService))
}

func TestHTTPService_APISubdomain_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		// Create a mock API
		api := coreTesting.NewMockAPI(t, "testapi").WithSubdomain("testsubdomain")
		core.RegisterAPI("testapi", api)

		// Call the APISubdomain method
		subdomain := httpService.APISubdomain("testapi", false)

		// Assertions
		assert.Equal(tb, "testsubdomain."+ctx.Config().Config().Core.Domain, subdomain)

		// Test with non-existent API
		subdomain = httpService.APISubdomain("nonexistent", false)
		assert.Empty(tb, subdomain)
	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService))
}

func TestHTTPService_APICatalog_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		httpService := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
		require.NotNil(tb, httpService)

		protocol := "http"
		if ctx.Config().Config().Core.Secure {
			protocol = "https"
		}
		rootURL := fmt.Sprintf("%s://%s", protocol, ctx.Config().Config().Core.Domain)

		// The catalog is a global path, so it must resolve regardless of the
		// Host header (root domain and any API subdomain alike).
		req := httptest.NewRequest(http.MethodGet, "/.well-known/api-catalog", nil)
		req.Host = "api." + ctx.Config().Config().Core.Domain
		rec := httptest.NewRecorder()
		httpService.Router().ServeHTTP(rec, req)

		require.Equal(tb, http.StatusOK, rec.Code)
		assert.Equal(tb, "application/linkset+json", rec.Header().Get("Content-Type"))

		var catalog struct {
			Linkset []struct {
				Anchor      string `json:"anchor"`
				ServiceDesc []struct {
					Href string `json:"href"`
					Type string `json:"type"`
				} `json:"service-desc"`
			} `json:"linkset"`
		}
		require.NoError(tb, json.Unmarshal(rec.Body.Bytes(), &catalog))
		require.NotEmpty(tb, catalog.Linkset)

		// The root domain entry must point at the root OpenAPI spec.
		foundRoot := false
		for _, entry := range catalog.Linkset {
			if entry.Anchor != rootURL {
				continue
			}
			foundRoot = true
			require.NotEmpty(tb, entry.ServiceDesc)
			assert.Equal(tb, rootURL+"/swagger.json", entry.ServiceDesc[0].Href)
			assert.Equal(tb, "application/vnd.oai.openapi+json;version=3.0", entry.ServiceDesc[0].Type)
		}
		assert.True(tb, foundRoot, "catalog should contain the root domain entry")

		// Follow each service-desc href to ensure it resolves to a live OpenAPI
		// spec, not a dead/incorrect link. The spec endpoint serves the document
		// as application/json, so validate the body parses as an OpenAPI doc.
		for _, entry := range catalog.Linkset {
			for _, sd := range entry.ServiceDesc {
				u, parseErr := url.Parse(sd.Href)
				require.NoError(tb, parseErr)
				getReq := httptest.NewRequest(http.MethodGet, u.Path, nil)
				getReq.Host = u.Host
				getRec := httptest.NewRecorder()
				httpService.Router().ServeHTTP(getRec, getReq)
				require.Equal(tb, http.StatusOK, getRec.Code, "href should resolve: %s", sd.Href)

				var spec struct {
					OpenAPI string `json:"openapi"`
				}
				require.NoError(tb, json.Unmarshal(getRec.Body.Bytes(), &spec))
				assert.Contains(tb, spec.OpenAPI, "3.")
			}
		}
	}, coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService))
}
