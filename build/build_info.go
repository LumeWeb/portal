package build

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

// BuildInfo defines the interface for build information
type BuildInfo interface {
	GetVersion() string
	GetCommit() string
	GetBranch() string
	GetBuildTime() time.Time
	GetBuildHost() string
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
	BuildHost    string    `json:"buildHost"`
	GoVersion    string    `json:"goVersion"`
	Platform     string    `json:"platform"`
	Architecture string    `json:"architecture"`
}

type build struct{}

var Default BuildInfo = &build{}

func (b *build) GetVersion() string {
	if Version == "" {
		return "develop"
	}
	return Version
}

func (b *build) GetCommit() string {
	if GitCommit == "" {
		return "unknown"
	}
	return GitCommit
}

func (b *build) GetBranch() string {
	if GitBranch == "" {
		return "unknown"
	}
	return GitBranch
}

func (b *build) GetBuildTime() time.Time {
	if BuildTime == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, BuildTime)
	return t
}

func (b *build) GetBuildHost() string {
	if BuildHost == "" {
		return "unknown"
	}
	return BuildHost
}

func (b *build) GetGoVersion() string {
	return runtime.Version()
}

func (b *build) GetPlatform() string {
	return runtime.GOOS
}

func (b *build) GetArchitecture() string {
	return runtime.GOARCH
}

func (b *build) IsRelease() bool {
	return b.GetVersion() != "develop" && b.GetCommit() != "unknown"
}

func (b *build) String() string {
	return fmt.Sprintf("%s-%s (%s)",
		b.GetVersion(),
		b.GetCommit()[:8],
		b.GetBuildTime().Format(time.RFC3339))
}

func (b *build) Short() string {
	return fmt.Sprintf("%s-%s", b.GetVersion(), b.GetCommit()[:8])
}

func (b *build) JSON() (string, error) {
	data, err := json.MarshalIndent(b.Info(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal build info: %w", err)
	}
	return string(data), nil
}

func (b *build) Info() Info {
	return Info{
		Version:      b.GetVersion(),
		GitCommit:    b.GetCommit(),
		GitBranch:    b.GetBranch(),
		BuildTime:    b.GetBuildTime(),
		BuildHost:    b.GetBuildHost(),
		GoVersion:    b.GetGoVersion(),
		Platform:     b.GetPlatform(),
		Architecture: b.GetArchitecture(),
	}
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
