// File: core/meta.go
package core

import (
	"go.lumeweb.com/portal/build"
	"io/fs"
	"net/http"
)

// PortalMeta represents the portal metadata
type PortalMeta struct {
	Domain       string            `json:"domain"`
	Plugins      PortalMetaPlugins `json:"plugins"`
	FeatureFlags map[string]bool   `json:"feature_flags"`
	Build        build.Info        `json:"build"` // Core build info
}

type PortalMetaPlugins = map[string]PortalMetaPlugin

type PortalMetaPlugin struct {
	Build      build.Info     `json:"build,omitempty"` // Plugin build info
	Meta       map[string]any `json:"meta"`
	WebBundles []string       `json:"web_bundles"`
}

// PortalMetaBuilder interface for building portal metadata
type PortalMetaBuilder interface {
	// Core functionality
	AddFeatureFlag(key string, value bool) PortalMetaBuilder

	// Plugin management
	AddPlugin(key string) PortalMetaBuilder
	AddPluginBuildInfo(key string, buildInfo build.Info) PortalMetaBuilder
	AddPluginMeta(pluginKey string, metaKey string, metaValue any) PortalMetaBuilder
	AddPluginWebBundle(pluginKey string, bundleUri string) PortalMetaBuilder

	Build() *PortalMeta
}

type WebBundle struct {
	Files        fs.FS
	FSPrefix     string
	ManifestPath string
	TargetApps   []string
}

type webBundleOption func(*WebBundle) *WebBundle

func NewWebBundle(fs fs.FS, options ...webBundleOption) *WebBundle {
	bundle := &WebBundle{
		Files: fs,
	}

	for _, option := range options {
		bundle = option(bundle)
	}

	return bundle
}

func WithWebBundlePrefix(prefix string) webBundleOption {
	return func(wb *WebBundle) *WebBundle {
		wb.FSPrefix = prefix
		return wb
	}
}

func WithWebBundleManifestPath(path string) webBundleOption {
	return func(wb *WebBundle) *WebBundle {
		wb.ManifestPath = path
		return wb
	}
}

func WithWebBundleTargetApps(apps ...string) webBundleOption {
	return func(wb *WebBundle) *WebBundle {
		wb.TargetApps = apps
		return wb
	}
}

func NewWebBundles(bundles ...*WebBundle) []*WebBundle {
	return bundles
}

type webBundleLiveFs struct {
	httpFS http.FileSystem
}

// Open implements fs.FS.
func (f *webBundleLiveFs) Open(name string) (fs.File, error) {
	// http.FileSystem expects slash-separated paths, just like fs.FS.
	// No path cleaning needed here usually, as fs.FS callers should provide valid paths.

	// Open the file using the underlying http.FileSystem.
	file, err := f.httpFS.Open(name)
	if err != nil {
		return nil, err // Propagate the error (e.g., os.ErrNotExist)
	}

	// The returned 'file' is of type http.File.
	// Since the http.File interface requires Read, Stat, and Close methods,
	// it implicitly satisfies the fs.File interface.
	// We can directly return it as an fs.File.
	return file, nil
}

func NewWebBundleLiveFS(path string) fs.FS {
	_fs := http.Dir(path)

	return &webBundleLiveFs{httpFS: _fs}
}
