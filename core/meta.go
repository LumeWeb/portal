// File: core/meta.go
package core

import (
	"go.lumeweb.com/portal/build"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
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
	Meta       map[string]any `json:"meta"`            // Arbitrary plugin metadata
	WebBundles []string       `json:"web_bundles"`     // List of web bundle URIs
}

// PortalMetaBuilder interface for building portal metadata
type PortalMetaBuilder interface {
	// Core functionality
	AddFeatureFlag(key string, value bool) PortalMetaBuilder
	AddCoreBuildInfo(buildInfo build.Info) PortalMetaBuilder

	// Plugin management
	AddPlugin(pluginID string) (PluginMetaBuilder, error)
	Build() *PortalMeta
}

// PluginMetaBuilder interface for building metadata for a specific plugin
type PluginMetaBuilder interface {
	AddBuildInfo(buildInfo build.Info) PluginMetaBuilder
	AddMeta(key string, value any) PluginMetaBuilder
	AddWebBundle(bundleURI string) PluginMetaBuilder
}

type WebBundle struct {
	Files        fs.FS
	FSPrefix     string
	ManifestPath string
	TargetApps   []string
}

type WebBundleOption func(*WebBundle) *WebBundle

func NewWebBundle(fs fs.FS, options ...WebBundleOption) *WebBundle {
	bundle := &WebBundle{
		Files: fs,
	}

	for _, option := range options {
		bundle = option(bundle)
	}

	return bundle
}

func WithWebBundlePrefix(prefix string) WebBundleOption {
	return func(wb *WebBundle) *WebBundle {
		wb.FSPrefix = prefix
		return wb
	}
}

func WithWebBundleManifestPath(path string) WebBundleOption {
	return func(wb *WebBundle) *WebBundle {
		wb.ManifestPath = path
		return wb
	}
}

func WithWebBundleTargetApps(apps ...string) WebBundleOption {
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

func (f *webBundleLiveFs) Open(name string) (fs.File, error) {
	// Clean the path and remove any leading/trailing slashes
	name = path.Clean(name)
	if strings.Contains(name, "..") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	file, err := f.httpFS.Open(strings.TrimPrefix(name, "/"))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func NewWebBundleLiveFS(path string) fs.FS {
	if info, err := os.Stat(path); err != nil {
		return &webBundleLiveFs{httpFS: http.Dir(path)}
	} else if !info.IsDir() {
		return &webBundleLiveFs{httpFS: http.Dir(path)}
	}
	return &webBundleLiveFs{httpFS: http.Dir(path)}
}
