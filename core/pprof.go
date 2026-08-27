package core

import (
	"context"

	"go.lumeweb.com/portal/config"
)

// PPROF_SERVICE identifies the service that provides the file-watcher based
// pprof dump feature described in config.ObservabilityConfig.Debug.
const PPROF_SERVICE = "pprof"

// PprofDebugService provides a fallback access point to the admin pprof debug
// tooling that bypasses the HTTP layer. When the file watcher detects a
// landmark file in the configured watch path, it dumps a full set of runtime
// pprof profiles and an execution trace to a predefined output directory.
type PprofDebugService interface {
	Service

	// DumpProfiles captures a full set of pprof profiles (CPU, heap, allocs,
	// goroutine, threadcreate, block, mutex) plus an execution trace, packaged
	// into a single zip file placed in cfg.OutputDir.
	DumpProfiles(ctx context.Context, cfg config.DebugConfig) error
}
