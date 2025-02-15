package web_manifest

// StatsAssets represents the assets structure
type StatsAssets struct {
	JS  AssetType `json:"js"`
	CSS AssetType `json:"css"`
}

// AssetType represents async and sync assets
type AssetType struct {
	Async []string `json:"async"`
	Sync  []string `json:"sync"`
}

// BuildInfo represents build information
type BuildInfo struct {
	BuildVersion string `json:"buildVersion"`
	BuildName    string `json:"buildName"`
}

// EntryInfo represents remote entry information
type EntryInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// MetaData represents metadata information
type MetaData struct {
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	BuildInfo      BuildInfo `json:"buildInfo"`
	RemoteEntry    EntryInfo `json:"remoteEntry"`
	SSRRemoteEntry EntryInfo `json:"ssrRemoteEntry"`
	Types          struct {
		Path string `json:"path"`
		Name string `json:"name"`
	} `json:"types"`
	GlobalName    string `json:"globalName"`
	PluginVersion string `json:"pluginVersion"`
	PublicPath    string `json:"publicPath"`
}

// ManifestShared represents shared dependencies
type ManifestShared struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	RequiredVersion string      `json:"requiredVersion"`
	Assets          StatsAssets `json:"assets"`
}

// ManifestExpose represents exposed modules
type ManifestExpose struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Assets StatsAssets `json:"assets"`
	Path   string      `json:"path"`
}

// RemoteWithEntry represents a remote with an entry point
type RemoteWithEntry struct {
	Entry                   string `json:"entry"`
	FederationContainerName string `json:"federationContainerName"`
	ModuleName              string `json:"moduleName"`
	Alias                   string `json:"alias"`
}

// RemoteWithVersion represents a remote with a version
type RemoteWithVersion struct {
	Version                 string `json:"version"`
	FederationContainerName string `json:"federationContainerName"`
	ModuleName              string `json:"moduleName"`
	Alias                   string `json:"alias"`
}

// ManifestRemote represents either a RemoteWithEntry or RemoteWithVersion
type ManifestRemote struct {
	// Using omitempty to handle the union type nature
	Entry                   string `json:"entry,omitempty"`
	Version                 string `json:"version,omitempty"`
	FederationContainerName string `json:"federationContainerName"`
	ModuleName              string `json:"moduleName"`
	Alias                   string `json:"alias"`
}

// Manifest represents the root structure
type Manifest struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	MetaData MetaData         `json:"metaData"`
	Shared   []ManifestShared `json:"shared"`
	Remotes  []ManifestRemote `json:"remotes"`
	Exposes  []ManifestExpose `json:"exposes"`
}
