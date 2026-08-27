package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

var _ core.PprofDebugService = (*PprofDebugServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.PPROF_SERVICE,
		Factory: NewPprofDebugService,
	})
}

// PprofDebugServiceDefault implements the file-watcher based pprof dump
// feature. It watches a configured directory for a landmark file and, when
// one appears, captures a full set of runtime pprof profiles and an execution
// trace to a predefined output directory. It serves as a fallback access
// point to the admin pprof debug tooling that does not depend on the HTTP
// layer.
type PprofDebugServiceDefault struct {
	*core.BaseComponent

	watcher *fsnotify.Watcher
	mu      sync.Mutex
}

func NewPprofDebugService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &PprofDebugServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(svc.start),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			if svc.watcher != nil {
				return svc.watcher.Close()
			}
			return nil
		}),
	)

	return svc, opts, nil
}

func (p *PprofDebugServiceDefault) ID() string {
	return core.PPROF_SERVICE
}

// DumpProfiles captures a full set of pprof profiles and an execution trace,
// packaged into a single zip file in cfg.OutputDir.
func (p *PprofDebugServiceDefault) DumpProfiles(ctx context.Context, cfg config.DebugConfig) error {
	return dumpProfiles(ctx, p.Logger(), cfg)
}

// start launches the fsnotify watcher when the debug feature is enabled.
func (p *PprofDebugServiceDefault) start(ctx core.Context) error {
	cfg := ctx.Config().Config().Core.Observability.Debug
	if !cfg.Enabled {
		return nil
	}

	watchPath := resolveWatchPath(cfg)
	if err := os.MkdirAll(watchPath, 0o755); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(watchPath); err != nil {
		watcher.Close()
		return err
	}
	p.watcher = watcher

	p.Logger().Info("pprof debug watcher started",
		zap.String("watch_path", watchPath),
		zap.String("landmark_file", cfg.LandmarkFile))

	go p.watchLoop(cfg)

	return nil
}

// watchLoop consumes filesystem events until the watcher is closed.
func (p *PprofDebugServiceDefault) watchLoop(cfg config.DebugConfig) {
	marker := filepath.Join(resolveWatchPath(cfg), cfg.LandmarkFile)

	for {
		select {
		case event, ok := <-p.watcher.Events:
			if !ok {
				return
			}
			if event.Name == marker && event.Op&fsnotify.Create != 0 {
				p.trigger(cfg)
			}
		case err, ok := <-p.watcher.Errors:
			if !ok {
				return
			}
			p.Logger().Error("pprof debug watcher error", zap.Error(err))
		}
	}
}

// trigger runs a single pprof dump, serializing concurrent triggers, then
// removes the landmark file so a freshly dropped file re-arms the watcher.
func (p *PprofDebugServiceDefault) trigger(cfg config.DebugConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Logger().Info("landmark file detected, dumping pprof data",
		zap.String("landmark_file", cfg.LandmarkFile))

	if err := p.DumpProfiles(p.Context().GetContext(), cfg); err != nil {
		p.Logger().Error("failed to dump pprof data", zap.Error(err))
	}

	// Consume the landmark so repeated dumps require a new file.
	marker := filepath.Join(resolveWatchPath(cfg), cfg.LandmarkFile)
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		p.Logger().Warn("failed to remove landmark file",
			zap.String("path", marker), zap.Error(err))
	}
}

// dumpProfiles enables full mutex/block capture ("set calls to 1"), samples
// the CPU and execution trace for ProfileSeconds, writes the static profiles
// to a per-dump directory, then packages everything into a single zip file
// which is the sole artifact left in the output directory.
func dumpProfiles(ctx context.Context, log *core.Logger, cfg config.DebugConfig) error {
	dir := resolveOutputDir(cfg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	createdAt := time.Now().UTC().Format("20060102-150405")
	stagingDir := filepath.Join(dir, "portal-pprof-"+createdAt)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return err
	}
	// Remove the staging dir on every return path (including a cancelled
	// dump) so a partial capture never leaks into the output directory.
	defer os.RemoveAll(stagingDir)

	// Capture all mutex and blocking events rather than a statistical sample.
	// These are process-global switches, so reset them once the dump completes
	// to avoid carrying sustained overhead for the process lifetime. Note: Go
	// exposes no reset API for the block/mutex accumulators, so consecutive
	// dumps may carry forward previously-reported contention events; this is
	// accepted for an opt-in debug fallback.
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	defer runtime.SetMutexProfileFraction(0)
	defer runtime.SetBlockProfileRate(0)

	secs := cfg.ProfileSeconds
	if secs <= 0 {
		secs = 30
	}
	window := time.Duration(secs) * time.Second

	// Time-windowed CPU profile and execution trace. Abort the whole dump if
	// the context is cancelled so a truncated capture is never packaged and
	// reported as a successful full dump.
	if err := captureWindow(ctx, log, filepath.Join(stagingDir, fmt.Sprintf("cpu-%s.pprof", createdAt)), window,
		func(w io.Writer) (captureStopper, error) {
			if err := pprof.StartCPUProfile(w); err != nil {
				return nil, err
			}
			return pprof.StopCPUProfile, nil
		}); isCancelled(err) {
		return err
	}
	if err := captureWindow(ctx, log, filepath.Join(stagingDir, fmt.Sprintf("trace-%s.out", createdAt)), window,
		func(w io.Writer) (captureStopper, error) {
			if err := trace.Start(w); err != nil {
				return nil, err
			}
			return trace.Stop, nil
		}); isCancelled(err) {
		return err
	}

	// Static profile snapshots.
	for _, name := range []string{"heap", "allocs", "goroutine", "threadcreate", "block", "mutex"} {
		writeProfile(log, stagingDir, name, createdAt)
	}

	// Package the staged files into a single zip and remove the loose files.
	zipPath := filepath.Join(dir, "portal-pprof-"+createdAt+".zip")
	if err := zipDir(stagingDir, zipPath); err != nil {
		log.Error("failed to zip pprof data", zap.Error(err))
		return err
	}

	log.Info("pprof data dumped",
		zap.String("zip_path", zipPath))
	return nil
}

// captureStopper signals the end of a capture started by a CaptureStarter.
type captureStopper func()

// CaptureStarter begins a time-windowed capture writing to the given writer
// and returns the function that stops the capture.
type CaptureStarter func(w io.Writer) (stop captureStopper, err error)

// captureWindow writes a time-windowed sample (CPU profile or execution
// trace) to the given file. begin starts the capture and must return a stop
// function; the capture runs for the full window before being stopped, or is
// cut short with ctx.Err() if ctx is cancelled.
func captureWindow(ctx context.Context, log *core.Logger, path string, window time.Duration, begin CaptureStarter) error {
	f, err := os.Create(path)
	if err != nil {
		log.Error("failed to create capture file",
			zap.String("file", path), zap.Error(err))
		return err
	}
	defer f.Close()

	stop, err := begin(f)
	if err != nil {
		log.Error("failed to start capture",
			zap.String("file", path), zap.Error(err))
		return err
	}

	select {
	case <-ctx.Done():
		stop()
		return ctx.Err()
	case <-time.After(window):
	}
	stop()
	return nil
}

// isCancelled reports whether err indicates context cancellation or a
// deadline, as opposed to a best-effort capture setup failure.
func isCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// writeProfile writes a single named runtime profile to a directory.
func writeProfile(log *core.Logger, dir, name, createdAt string) {
	profile := pprof.Lookup(name)
	if profile == nil {
		log.Debug("profile not available, skipping",
			zap.String("profile", name))
		return
	}

	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s.pprof", name, createdAt)))
	if err != nil {
		log.Error("failed to create profile file",
			zap.String("profile", name), zap.Error(err))
		return
	}
	defer f.Close()

	if err := profile.WriteTo(f, 0); err != nil {
		log.Error("failed to write profile",
			zap.String("profile", name), zap.Error(err))
	}
}

// zipDir archives all files in srcDir into a new zip file at zipPath.
func zipDir(srcDir, zipPath string) error {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate

		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(w, f)
		return err
	})
}

func resolveWatchPath(cfg config.DebugConfig) string {
	if cfg.WatchPath != "" {
		return cfg.WatchPath
	}
	return os.TempDir()
}

func resolveOutputDir(cfg config.DebugConfig) string {
	if cfg.OutputDir != "" {
		return cfg.OutputDir
	}
	return filepath.Join(os.TempDir(), "portal-pprof")
}
