package service_tests

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/service"
)

// TestPprofDebugService_DumpProfilesCreatesZip verifies that DumpProfiles
// packages every pprof profile plus the trace into a single zip file.
func TestPprofDebugService_DumpProfilesCreatesZip(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PprofDebugService](ctx, core.PPROF_SERVICE)
		require.NotNil(tb, svc)

		out := tb.TempDir()
		cfg := config.DebugConfig{Enabled: true, OutputDir: out, ProfileSeconds: 1}

		require.NoError(tb, svc.DumpProfiles(context.Background(), cfg))

		zipPath := findZip(tb, out)
		require.NotEmpty(tb, zipPath, "expected a zip file in output dir")

		entries := zipEntries(tb, zipPath)
		for _, want := range []string{"cpu-", "trace-", "heap-", "allocs-", "goroutine-", "threadcreate-", "block-", "mutex-"} {
			assert.True(tb, hasPrefix(entries, want), "expected zip entry with prefix %q, got %v", want, entries)
		}
	}, coreTesting.WithServiceFactory(core.PPROF_SERVICE, service.NewPprofDebugService))
}

// TestPprofDebugService_DumpProfilesCancelledNoZip verifies that a cancelled
// context aborts the dump instead of packaging a truncated capture as success.
func TestPprofDebugService_DumpProfilesCancelledNoZip(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PprofDebugService](ctx, core.PPROF_SERVICE)
		require.NotNil(tb, svc)

		out := tb.TempDir()
		cfg := config.DebugConfig{Enabled: true, OutputDir: out, ProfileSeconds: 1}

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		err := svc.DumpProfiles(cancelled, cfg)
		require.Error(tb, err)
		assert.ErrorIs(tb, err, context.Canceled)
		assert.Empty(tb, findZip(tb, out), "no zip expected when dump is cancelled")
	}, coreTesting.WithServiceFactory(core.PPROF_SERVICE, service.NewPprofDebugService))
}

// TestPprofDebugService_WatcherTriggersDump verifies the end-to-end flow: when
// the file watcher detects the landmark file it dumps the pprof data and
// consumes the landmark so a new file re-arms the watcher.
func TestPprofDebugService_WatcherTriggersDump(t *testing.T) {
	watch := t.TempDir()
	out := t.TempDir()

	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		marker := filepath.Join(watch, "portal.pprof")
		require.NoError(tb, os.WriteFile(marker, []byte("trigger"), 0o644))

		require.Eventually(tb, func() bool {
			return findZip(tb, out) != ""
		}, 20*time.Second, 200*time.Millisecond)

		_, err := os.Stat(marker)
		assert.True(tb, os.IsNotExist(err), "landmark file should be removed after dump")
	},
		coreTesting.WithServiceFactory(core.PPROF_SERVICE, service.NewPprofDebugService),
		coreTesting.WithConfig("core.observability.debug.enabled", true),
		coreTesting.WithConfig("core.observability.debug.watch_path", watch),
		coreTesting.WithConfig("core.observability.debug.output_dir", out),
		coreTesting.WithConfig("core.observability.debug.landmark_file", "portal.pprof"),
		coreTesting.WithConfig("core.observability.debug.profile_seconds", 1),
	)
}

// TestPprofDebugService_DisabledDoesNotStartWatcher verifies that with the
// feature disabled, dropping a landmark file does not produce a dump.
func TestPprofDebugService_DisabledDoesNotStartWatcher(t *testing.T) {
	watch := t.TempDir()
	out := t.TempDir()

	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		marker := filepath.Join(watch, "portal.pprof")
		require.NoError(tb, os.WriteFile(marker, []byte("trigger"), 0o644))

		// Give any (incorrectly started) watcher a window to act; no dump
		// should appear while the feature is disabled.
		time.Sleep(2 * time.Second)
		assert.Empty(tb, findZip(tb, out), "no dump expected when disabled")
	},
		coreTesting.WithServiceFactory(core.PPROF_SERVICE, service.NewPprofDebugService),
		coreTesting.WithConfig("core.observability.debug.enabled", false),
		coreTesting.WithConfig("core.observability.debug.watch_path", watch),
		coreTesting.WithConfig("core.observability.debug.output_dir", out),
		coreTesting.WithConfig("core.observability.debug.landmark_file", "portal.pprof"),
	)
}

func findZip(tb coreTesting.TB, dir string) string {
	tb.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	require.NoError(tb, err)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func zipEntries(tb coreTesting.TB, zipPath string) []string {
	tb.Helper()
	r, err := zip.OpenReader(zipPath)
	require.NoError(tb, err)
	defer r.Close()

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		require.NoError(tb, err)
		_, _ = io.Copy(io.Discard, rc)
		rc.Close()
	}
	return names
}

func hasPrefix(entries []string, prefix string) bool {
	for _, e := range entries {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
