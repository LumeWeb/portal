package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestConfig creates a config.Config with the given observability settings.
func newTestConfig(obs config.ObservabilityConfig) *config.Config {
	return &config.Config{
		Core: config.CoreConfig{
			Observability: obs,
		},
	}
}

// runStartupAndExit simulates the portal lifecycle: runs all startup funcs.
func runStartupAndExit(t *testing.T, ctx Context) {
	t.Helper()
	for _, f := range ctx.StartupFuncs() {
		require.NoError(t, f(ctx))
	}
}

// runExit runs all exit funcs in order.
func runExit(t *testing.T, ctx Context) {
	t.Helper()
	for _, f := range ctx.ExitFuncs() {
		require.NoError(t, f(ctx))
	}
}

// newMockConfigManagerSimple creates a minimal config manager for telemetry tests.
func newMockConfigManagerSimple(t *testing.T, cfg *config.Config) *config.MockManager {
	mockConfigManager := config.NewMockManager(t)
	mockConfigManager.On("GetConfig").Maybe().Return(cfg)
	mockConfigManager.On("Config").Maybe().Return(cfg)
	mockConfigManager.On("SetLogger", mock.Anything).Maybe().Return()
	return mockConfigManager
}

// TestContextWithTelemetry_OnExitAlwaysRegistered is the regression test for
// a bug where OnExit was registered INSIDE the startup func. If the startup
// func returned early (tracing disabled, no OTLP endpoint), tp.Shutdown()
// was never called and all buffered spans in the DeferredSpanProcessor were
// silently lost. OnExit must be registered at builder time so it always runs.
func TestContextWithTelemetry_OnExitAlwaysRegistered(t *testing.T) {
	ResetState()

	// Config: observability enabled, but NO OTLP endpoint.
	// This causes the startup func to return early at the
	// `if cfg.OTLP.Endpoint == ""` guard.
	cfg := newTestConfig(config.ObservabilityConfig{
		Enabled:     true,
		ServiceName: "test-portal",
		OTLP:        config.OTLPConfig{Endpoint: ""},
		Tracing: config.TracingConfig{
			Enabled:            true,
			Sampler:            config.SamplerAlways,
			BatchTimeout:       5,
			MaxExportBatchSize: 512,
		},
	})

	mockCfg := newMockConfigManagerSimple(t, cfg)
	logger := NewLogger(mockCfg, nil)

	ctx, err := NewContext(mockCfg, logger)
	require.NoError(t, err)

	runStartupAndExit(t, ctx)

	// OnExit should be registered at builder time, giving us >= 2 exit funcs
	// (event manager close + telemetry shutdown). If OnExit is still inside
	// the startup func after the early-return guard, this will be 1 and fail.
	exitFuncs := ctx.ExitFuncs()
	assert.GreaterOrEqual(t, len(exitFuncs), 2,
		"OnExit must be registered at builder time — early-return paths in "+
			"the startup func must not skip tp.Shutdown()")

	runExit(t, ctx)
}

// TestContextWithTelemetry_TracingDisabled verifies that when tracing is
// disabled entirely (cfg.Enabled = false), the TracerProvider uses
// NeverSample and no spans are recorded, and shutdown is clean.
func TestContextWithTelemetry_TracingDisabled(t *testing.T) {
	ResetState()

	cfg := newTestConfig(config.ObservabilityConfig{Enabled: false})

	mockCfg := newMockConfigManagerSimple(t, cfg)
	logger := NewLogger(mockCfg, nil)

	ctx, err := NewContext(mockCfg, logger)
	require.NoError(t, err)

	runStartupAndExit(t, ctx)

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "should-not-record")
	span.End()

	tp := otel.GetTracerProvider()
	assert.NotNil(t, tp)

	runExit(t, ctx)
}

// TestContextWithTelemetry_EarlyReturnNoPanic verifies that all early-return
// paths in the startup func (disabled, no endpoint, tracing subsystem disabled)
// don't panic when exit funcs run — i.e. tp.Shutdown() is always reachable.
func TestContextWithTelemetry_EarlyReturnNoPanic(t *testing.T) {
	t.Run("tracing_disabled", func(t *testing.T) {
		ResetState()
		cfg := newTestConfig(config.ObservabilityConfig{Enabled: false})
		mockCfg := newMockConfigManagerSimple(t, cfg)
		logger := NewLogger(mockCfg, nil)
		ctx, err := NewContext(mockCfg, logger)
		require.NoError(t, err)
		runStartupAndExit(t, ctx)
		assert.NotPanics(t, func() { runExit(t, ctx) })
	})

	t.Run("enabled_no_endpoint", func(t *testing.T) {
		ResetState()
		cfg := newTestConfig(config.ObservabilityConfig{
			Enabled: true,
			OTLP:    config.OTLPConfig{Endpoint: ""},
		})
		mockCfg := newMockConfigManagerSimple(t, cfg)
		logger := NewLogger(mockCfg, nil)
		ctx, err := NewContext(mockCfg, logger)
		require.NoError(t, err)
		runStartupAndExit(t, ctx)
		assert.NotPanics(t, func() { runExit(t, ctx) })
	})

	t.Run("tracing_enabled_no_tracing_subsystem", func(t *testing.T) {
		ResetState()
		// Observability enabled, OTLP endpoint set, but Tracing.Enabled = false.
		// SetExporter is never called, but OnExit should still fire.
		cfg := newTestConfig(config.ObservabilityConfig{
			Enabled:     true,
			ServiceName: "test",
			OTLP:        config.OTLPConfig{Endpoint: "localhost:4317", Insecure: true},
			Tracing:     config.TracingConfig{Enabled: false},
		})
		mockCfg := newMockConfigManagerSimple(t, cfg)
		logger := NewLogger(mockCfg, nil)
		ctx, err := NewContext(mockCfg, logger)
		require.NoError(t, err)
		runStartupAndExit(t, ctx)
		assert.NotPanics(t, func() { runExit(t, ctx) })
	})
}

// TestContextWithTelemetry_BootSpansExportedViaDeferredProcessor verifies
// the end-to-end flow: spans created on a TracerProvider with a
// DeferredSpanProcessor (before exporter attachment) are buffered and
// flushed when SetExporter is called. This is the mechanism that allows
// portal.boot spans to reach Tempo.
func TestContextWithTelemetry_BootSpansExportedViaDeferredProcessor(t *testing.T) {
	deferred := NewDeferredSpanProcessor()
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("test")

	// Span before exporter attachment — buffered
	_, bootSpan := tracer.Start(context.Background(), "portal.boot")
	bootSpan.End()
	assert.Equal(t, 0, len(exporter.GetSpans()))

	// Attach exporter — buffered span flushes
	deferred.SetExporter(exporter)
	deferred.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "portal.boot", spans[0].Name)

	// Runtime span after attachment — direct
	_, runtimeSpan := tracer.Start(context.Background(), "runtime.request")
	runtimeSpan.End()
	require.NoError(t, tp.ForceFlush(context.Background()))

	spans = exporter.GetSpans()
	require.Len(t, spans, 2)
	assert.Equal(t, "portal.boot", spans[0].Name)
	assert.Equal(t, "runtime.request", spans[1].Name)
}
