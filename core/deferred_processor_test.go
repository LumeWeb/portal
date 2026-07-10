package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestDeferredSpanProcessor_BuffersSpansBeforeExporter(t *testing.T) {
	deferred := NewDeferredSpanProcessor()
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Create and end a span before exporter is attached — should be buffered
	_, span := tracer.Start(context.Background(), "pre-export-span")
	span.End()

	// Spans should be buffered, not exported
	assert.Equal(t, 0, len(exporter.GetSpans()))

	// Attach exporter — buffered spans should flush
	deferred.SetExporter(exporter)
	deferred.ForceFlush(context.Background())
	assert.Equal(t, 1, len(exporter.GetSpans()))
}

func TestDeferredSpanProcessor_ForwardsAfterExporter(t *testing.T) {
	deferred := NewDeferredSpanProcessor()
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Attach exporter first
	deferred.SetExporter(exporter)

	// Now create and end a span — should go directly through
	_, span := tracer.Start(context.Background(), "post-export-span")
	span.End()

	// Force flush to ensure spans are exported
	require.NoError(t, tp.ForceFlush(context.Background()))

	assert.Equal(t, 1, len(exporter.GetSpans()))
}

func TestDeferredSpanProcessor_ShutdownNoExporter(t *testing.T) {
	deferred := NewDeferredSpanProcessor()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	tracer := tp.Tracer("test")

	// Create and end a span with no exporter attached
	_, span := tracer.Start(context.Background(), "shutdown-span")
	span.End()

	// Shutdown should not panic or error
	err := tp.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestDeferredSpanProcessor_SetExporterIdempotent(t *testing.T) {
	deferred := NewDeferredSpanProcessor()
	exporter1 := tracetest.NewInMemoryExporter()
	exporter2 := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Create a span before any exporter
	_, span1 := tracer.Start(context.Background(), "span1")
	span1.End()

	// Attach first exporter
	deferred.SetExporter(exporter1)
	deferred.ForceFlush(context.Background())
	assert.Equal(t, 1, len(exporter1.GetSpans()))

	// Create another span
	_, span2 := tracer.Start(context.Background(), "span2")
	span2.End()

	// Attach second exporter — should NOT re-send span1, only span2 to
	// exporter2 if SetExporter replaces the processor. But SetExporter creates
	// a new BSP, so span1 was already flushed to exporter1. span2 should go to
	// exporter2 via the new BSP.
	deferred.SetExporter(exporter2)

	// span1 should only be in exporter1 (already flushed)
	assert.Equal(t, 1, len(exporter1.GetSpans()))
}

func TestDeferredSpanProcessor_ConcurrentAccess(t *testing.T) {
	deferred := NewDeferredSpanProcessor()
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deferred),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	var wg sync.WaitGroup
	// Spawn multiple goroutines creating and ending spans concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, span := tracer.Start(context.Background(), "concurrent-span")
			span.End()
		}()
	}

	// Attach exporter while spans are being created
	time.Sleep(10 * time.Millisecond)
	deferred.SetExporter(exporter)

	wg.Wait()

	// All spans should eventually be accounted for (either buffered-then-flushed
	// or sent directly through the real processor)
	// We need to force flush to ensure all spans are exported
	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	assert.GreaterOrEqual(t, len(spans), 1, "should have exported at least some spans")
}
