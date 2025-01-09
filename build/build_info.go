package build

import (
	"encoding/json"
	"fmt"
	"time"
)

var _ BuildInfo = &Info{}

// BuildInfo defines the interface for build information
type BuildInfo interface {
	GetVersion() string
	GetCommit() string
	GetBranch() string
	GetBuildTime() time.Time
	GetGoVersion() string
	GetPlatform() string
	GetArchitecture() string
	IsRelease() bool
	String() string
	Short() string
	JSON() (string, error)
	Info() Info
}

// Info contains all build-time information
type Info struct {
	Version      string    `json:"version"`
	GitCommit    string    `json:"gitCommit"`
	GitBranch    string    `json:"gitBranch"`
	BuildTime    time.Time `json:"buildTime"`
	GoVersion    string    `json:"goVersion"`
	Platform     string    `json:"platform"`
	Architecture string    `json:"architecture"`
}

var Default BuildInfo = New(Version, GitCommit, GitBranch, BuildTime, GoVersion, Platform, Architecture)

func (i *Info) GetCommit() string {
	if i.GitCommit == "" {
		return "unknown"
	}
	return i.GitCommit
}

func (i *Info) GetBranch() string {
	if i.GitBranch == "" {
		return "unknown"
	}
	return i.GitBranch
}

func (i *Info) GetBuildTime() time.Time {
	return i.BuildTime
}

func (i *Info) GetVersion() string {
	if i.Version == "" {
		return "develop"
	}
	return i.Version
}

func (i *Info) GetGoVersion() string {
	return i.GoVersion
}

func (i *Info) GetPlatform() string {
	return i.Platform
}

func (i *Info) GetArchitecture() string {
	return i.Architecture
}

func (i *Info) IsRelease() bool {
	return i.GetVersion() != "develop" && i.GetCommit() != "unknown"
}

func (i *Info) String() string {
	return fmt.Sprintf("%s-%s (%s)",
		i.GetVersion(),
		i.GetCommit()[:8],
		i.GetBuildTime().Format(time.RFC3339))
}

func (i *Info) Short() string {
	commit := i.GetCommit()
	if len(commit) > 8 {
		commit = commit[:8]
	}
	return fmt.Sprintf("%s-%s", i.GetVersion(), commit)
}

func (i *Info) JSON() (string, error) {
	data, err := json.MarshalIndent(i.Info(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal build info: %w", err)
	}
	return string(data), nil
}

func (i *Info) Info() Info {
	return *i
}

// Convenience package-level functions
func GetInfo() Info {
	return Default.Info()
}

func String() string {
	return Default.String()
}

func Short() string {
	return Default.Short()
}

func JSON() (string, error) {
	return Default.JSON()
}

func IsRelease() bool {
	return Default.IsRelease()
}

func New(version, commit, branch, buildTime, goVersion, platform, architecture string) BuildInfo {
	var bTime time.Time
	if buildTime != "" {
		bTime, _ = time.Parse(time.RFC3339, buildTime)
	}

	return &Info{
		Version:      version,
		GitCommit:    commit,
		GitBranch:    branch,
		BuildTime:    bTime,
		GoVersion:    goVersion,
		Platform:     platform,
		Architecture: architecture,
	}
}
