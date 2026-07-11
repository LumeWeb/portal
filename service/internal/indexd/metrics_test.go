package indexd

import (
	"bytes"
	"context"
	"io"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// resetMetrics resets all download metrics to a clean state for testing.
func resetMetrics() {
	DownloadDuration.Reset()
	DownloadBytes.Reset()
	DownloadErrors.Reset()
	DownloadActive.Reset()
}

func collectCounter(t *testing.T, cv prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	c, err := cv.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	m := &dto.Metric{}
	if err := c.(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("metric.Write: %v", err)
	}
	return m.GetCounter().GetValue()
}

func collectHistogram(t *testing.T, hv prometheus.HistogramVec, labels ...string) *dto.Histogram {
	t.Helper()
	h, err := hv.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	m := &dto.Metric{}
	if err := h.(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("metric.Write: %v", err)
	}
	return m.GetHistogram()
}

func collectGauge(t *testing.T, gv prometheus.GaugeVec, labels ...string) float64 {
	t.Helper()
	g, err := gv.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	m := &dto.Metric{}
	if err := g.(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("metric.Write: %v", err)
	}
	return m.GetGauge().GetValue()
}

// newTestTracedReadCloserWithMetrics creates a tracedReadCloser with both
// OTel span recording and Prometheus metrics.
func newTestTracedReadCloserWithMetrics(data []byte) (*tracedReadCloser, *tracetest.InMemoryExporter) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exp),
	)
	otel.SetTracerProvider(tp)
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	span.End()

	rc := &recordingReadCloser{data: bytes.NewReader(data)}
	return &tracedReadCloser{
		inner: rc,
		ctx:   ctx,
		objID: "test-object-id",
	}, exp
}

func TestMetrics_DownloadBytesRecorded(t *testing.T) {
	resetMetrics()
	payload := []byte("hello sia download world")
	trc, _ := newTestTracedReadCloserWithMetrics(payload)

	buf := make([]byte, len(payload))
	_, _ = trc.Read(buf)
	trc.Close()

	bytes := collectCounter(t, DownloadBytes, LabelOpDownload)
	if bytes != float64(len(payload)) {
		t.Fatalf("expected downloadBytes=%d, got %f", len(payload), bytes)
	}
}

func TestMetrics_DurationRecorded(t *testing.T) {
	resetMetrics()
	payload := bytes.Repeat([]byte("X"), 1024)
	trc, _ := newTestTracedReadCloserWithMetrics(payload)

	// Read all
	buf := make([]byte, 1024)
	_, _ = trc.Read(buf)
	trc.Close()

	hist := collectHistogram(t, DownloadDuration, LabelOpDownload)
	if hist.GetSampleCount() != 1 {
		t.Fatalf("expected 1 duration sample, got %d", hist.GetSampleCount())
	}
	// Duration should be very small (< 1s)
	if hist.GetSampleSum() > 1.0 {
		t.Fatalf("duration unexpectedly high: %f seconds", hist.GetSampleSum())
	}
}

func TestMetrics_ActiveGaugeIncrementsAndDecrements(t *testing.T) {
	resetMetrics()
	payload := []byte("data")
	trc, _ := newTestTracedReadCloserWithMetrics(payload)

	// Before Read — gauge should be 0
	g := collectGauge(t, DownloadActive, LabelOpDownload)
	if g != 0 {
		t.Fatalf("expected active=0 before Read, got %f", g)
	}

	// After Read — gauge should be 1
	buf := make([]byte, 4)
	_, _ = trc.Read(buf)
	g = collectGauge(t, DownloadActive, LabelOpDownload)
	if g != 1 {
		t.Fatalf("expected active=1 after Read, got %f", g)
	}

	// After Close — gauge should be back to 0
	trc.Close()
	g = collectGauge(t, DownloadActive, LabelOpDownload)
	if g != 0 {
		t.Fatalf("expected active=0 after Close, got %f", g)
	}
}

func TestMetrics_ErrorCounterIncremented(t *testing.T) {
	resetMetrics()
	trc, _ := newTestTracedReadCloserWithMetrics(nil)
	trc.inner = &errorReadCloser{err: io.ErrUnexpectedEOF}

	buf := make([]byte, 10)
	_, _ = trc.Read(buf)
	trc.Close()

	errs := collectCounter(t, DownloadErrors, LabelOpDownload)
	if errs != 1 {
		t.Fatalf("expected error count=1, got %f", errs)
	}
}

func TestMetrics_MultipleReadsSingleDurationSample(t *testing.T) {
	resetMetrics()
	payload := bytes.Repeat([]byte("A"), 1000)
	trc, _ := newTestTracedReadCloserWithMetrics(payload)

	// Read in small chunks
	buf := make([]byte, 10)
	for {
		_, err := trc.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}
	trc.Close()

	hist := collectHistogram(t, DownloadDuration, LabelOpDownload)
	if hist.GetSampleCount() != 1 {
		t.Fatalf("expected 1 duration sample for multiple reads, got %d", hist.GetSampleCount())
	}

	bytes := collectCounter(t, DownloadBytes, LabelOpDownload)
	if bytes != 1000 {
		t.Fatalf("expected downloadBytes=1000, got %f", bytes)
	}
}

func TestMetrics_CloseWithoutReadNoDuration(t *testing.T) {
	resetMetrics()
	trc, _ := newTestTracedReadCloserWithMetrics(nil)
	trc.Close()

	// startTime is set in Close's once.Do, so duration will be ~0
	// but bytes should be 0
	bytes := collectCounter(t, DownloadBytes, LabelOpDownload)
	if bytes != 0 {
		t.Fatalf("expected downloadBytes=0 for unread reader, got %f", bytes)
	}
}

// ensure codes import is used (reused from traced_reader_test.go but in case
// that file is not compiled in yet)
var _ = codes.Error
