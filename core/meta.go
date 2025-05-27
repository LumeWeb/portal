// File: core/meta.go
package core

import (
	"fmt"
	"go.lumeweb.com/portal/build"
	"io/fs"
	"net/http"
	"os"
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

// portalMetaBuilder implements PortalMetaBuilder
type portalMetaBuilder struct {
	meta *PortalMeta
}

// pluginMetaBuilder implements PluginMetaBuilder
type pluginMetaBuilder struct {
	meta       *PortalMeta
	pluginID   string
	pluginMeta PortalMetaPlugin
}

// NewPortalMetaBuilder creates a new PortalMetaBuilder instance
func NewPortalMetaBuilder(domain string) PortalMetaBuilder {
	return &portalMetaBuilder{
		meta: &PortalMeta{
			Domain:       domain,
			Plugins:      make(PortalMetaPlugins),
			FeatureFlags: make(map[string]bool),
		},
	}
}

func (b *portalMetaBuilder) AddFeatureFlag(key string, value bool) PortalMetaBuilder {
	b.meta.FeatureFlags[key] = value
	return b
}

func (b *portalMetaBuilder) AddCoreBuildInfo(buildInfo build.Info) PortalMetaBuilder {
	b.meta.Build = buildInfo
	return b
}

func (b *portalMetaBuilder) AddPlugin(pluginID string) (PluginMetaBuilder, error) {
	if _, exists := b.meta.Plugins[pluginID]; exists {
		return nil, fmt.Errorf("plugin %s already exists in meta", pluginID)
	}

	pluginMeta := PortalMetaPlugin{
		Meta:       make(map[string]any),
		WebBundles: make([]string, 0),
	}
	b.meta.Plugins[pluginID] = pluginMeta

	return &pluginMetaBuilder{
		meta:       b.meta,
		pluginID:   pluginID,
		pluginMeta: pluginMeta,
	}, nil
}

func (b *portalMetaBuilder) Build() *PortalMeta {
	return b.meta
}

func (p *pluginMetaBuilder) AddBuildInfo(buildInfo build.Info) PluginMetaBuilder {
	p.pluginMeta.Build = buildInfo
	p.meta.Plugins[p.pluginID] = p.pluginMeta
	return p
}

func (p *pluginMetaBuilder) AddMeta(key string, value any) PluginMetaBuilder {
	p.pluginMeta.Meta[key] = value
	return p
}

func (p *pluginMetaBuilder) AddWebBundle(bundleURI string) PluginMetaBuilder {
	p.pluginMeta.WebBundles = append(p.pluginMeta.WebBundles, bundleURI)
	p.meta.Plugins[p.pluginID] = p.pluginMeta
	return p
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
	// Validate path to prevent directory traversal
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

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
	// Validate that path exists and is a directory
	if info, err := os.Stat(path); err != nil {
		// Return a filesystem that will provide the actual error message
		return &webBundleLiveFs{httpFS: http.Dir(path)}
	} else if !info.IsDir() {
		// Return a filesystem that will provide a "not a directory" error
		return &webBundleLiveFs{httpFS: http.Dir(path)}
	}

	// Path is valid directory - return normal filesystem
	return &webBundleLiveFs{httpFS: http.Dir(path)}
}
