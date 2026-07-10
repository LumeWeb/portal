package core

import (
	"context"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// DeferredSpanProcessor is a SpanProcessor that buffers spans until a real
// exporter is attached via SetExporter. This allows spans created during early
// boot (before OTel config is wired) to be captured on a real TracerProvider
// and exported once the OTLP exporter becomes available.
//
// Spans that end before SetExporter is called are buffered. When SetExporter
// is called, the buffered spans are flushed to the real processor, and all
// subsequent spans go directly through it.
type DeferredSpanProcessor struct {
	mu       sync.Mutex
	real     sdktrace.SpanProcessor
	buffered []sdktrace.ReadOnlySpan
	shutdown bool
}

// NewDeferredSpanProcessor creates a DeferredSpanProcessor with no exporter
// attached. Spans will be buffered until SetExporter is called.
func NewDeferredSpanProcessor() *DeferredSpanProcessor {
	return &DeferredSpanProcessor{}
}

// SetExporter attaches a real SpanExporter by creating a BatchSpanProcessor
// and flushing any buffered spans to it. After this call, all new and
// buffered spans are exported through the real processor.
func (d *DeferredSpanProcessor) SetExporter(exp sdktrace.SpanExporter, opts ...sdktrace.BatchSpanProcessorOption) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.shutdown {
		return
	}

	d.real = sdktrace.NewBatchSpanProcessor(exp, opts...)

	// Flush buffered spans
	for _, span := range d.buffered {
		d.real.OnEnd(span)
	}
	d.buffered = nil
}

// OnStart implements SpanProcessor.OnStart.
func (d *DeferredSpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.real != nil {
		d.real.OnStart(parent, s)
	}
	// If no real processor yet, nothing to do — the span is live and
	// will be buffered on OnEnd.
}

// OnEnd implements SpanProcessor.OnEnd.
func (d *DeferredSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.shutdown {
		return
	}

	if d.real != nil {
		d.real.OnEnd(s)
	} else {
		d.buffered = append(d.buffered, s)
	}
}

// Shutdown implements SpanProcessor.Shutdown.
func (d *DeferredSpanProcessor) Shutdown(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.shutdown = true
	d.buffered = nil

	if d.real != nil {
		return d.real.Shutdown(ctx)
	}
	return nil
}

// ForceFlush implements SpanProcessor.ForceFlush.
func (d *DeferredSpanProcessor) ForceFlush(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.real != nil {
		return d.real.ForceFlush(ctx)
	}
	return nil
}
