package indexd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordingReadCloser wraps a bytes.Reader and tracks Read/Close calls.
type recordingReadCloser struct {
	data       *bytes.Reader
	readCalls  int
	closeCalls int
}

func (r *recordingReadCloser) Read(p []byte) (int, error) {
	r.readCalls++
	return r.data.Read(p)
}

func (r *recordingReadCloser) Close() error {
	r.closeCalls++
	return nil
}

// newTestTracedReadCloser creates a tracedReadCloser with a real OTel span
// recorder so we can inspect the spans it emits.
func newTestTracedReadCloser(data []byte) (*tracedReadCloser, *tracetest.InMemoryExporter) {
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

func TestTracedReadCloser_ReadAllData(t *testing.T) {
	payload := []byte("hello sia download")
	trc, exp := newTestTracedReadCloser(payload)

	// Read all data in a single buffer.
	buf := make([]byte, len(payload))
	n, err := trc.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes, got %d", len(payload), n)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("data mismatch: got %q, want %q", buf, payload)
	}

	// Close should end the span.
	if err := trc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	spans := exp.GetSpans()
	var s tracetest.SpanStub
	for _, sp := range spans {
		if sp.Name == "indexd.SDKAdapter.Download.Read" {
			s = sp
			break
		}
	}
	if s.Name == "" {
		t.Fatalf("indexd.SDKAdapter.Download.Read span not found (spans: %v)", spanNames(spans))
	}
	// Verify downloadBytes attribute.
	var foundBytes bool
	for _, attr := range s.Attributes {
		if attr.Key == "indexd.downloadBytes" {
			foundBytes = true
			if attr.Value.AsInt64() != int64(len(payload)) {
				t.Fatalf("expected downloadBytes=%d, got %d", len(payload), attr.Value.AsInt64())
			}
		}
		if attr.Key == "indexd.siaObjectID" {
			if attr.Value.AsString() != "test-object-id" {
				t.Fatalf("expected siaObjectID='test-object-id', got %q", attr.Value.AsString())
			}
		}
	}
	if !foundBytes {
		t.Fatal("indexd.downloadBytes attribute not found on span")
	}
}

func spanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}

func filterSpansByName(spans []tracetest.SpanStub, name string) []tracetest.SpanStub {
	var filtered []tracetest.SpanStub
	for _, s := range spans {
		if s.Name == name {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func TestTracedReadCloser_MultiReadSingleSpan(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 100)
	trc, exp := newTestTracedReadCloser(payload)

	// Read in small chunks — should still produce exactly one span.
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

	spans := exp.GetSpans()
	downloadSpans := filterSpansByName(spans, "indexd.SDKAdapter.Download.Read")
	if len(downloadSpans) != 1 {
		t.Fatalf("expected exactly 1 Download.Read span (started on first Read, ended on Close), got %d", len(downloadSpans))
	}

	// Verify total bytes tracked.
	for _, attr := range downloadSpans[0].Attributes {
		if attr.Key == "indexd.downloadBytes" {
			if attr.Value.AsInt64() != 100 {
				t.Fatalf("expected downloadBytes=100, got %d", attr.Value.AsInt64())
			}
		}
	}
}

func TestTracedReadCloser_CloseWithoutRead(t *testing.T) {
	// If the reader is closed without any Read calls, a span should still
	// be emitted (with 0 bytes) so it's visible in traces.
	trc, exp := newTestTracedReadCloser(nil)

	if err := trc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	spans := exp.GetSpans()
	downloadSpans := filterSpansByName(spans, "indexd.SDKAdapter.Download.Read")
	if len(downloadSpans) != 1 {
		t.Fatalf("expected 1 Download.Read span even with no reads, got %d", len(downloadSpans))
	}
	for _, attr := range downloadSpans[0].Attributes {
		if attr.Key == "indexd.downloadBytes" {
			if attr.Value.AsInt64() != 0 {
				t.Fatalf("expected downloadBytes=0 for unread reader, got %d", attr.Value.AsInt64())
			}
		}
	}
}

func TestTracedReadCloser_PassThroughData(t *testing.T) {
	// Verify the wrapper passes data through correctly in both directions.
	payload := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	trc, _ := newTestTracedReadCloser(payload)
	defer trc.Close()

	out, err := io.ReadAll(trc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatalf("data mismatch: got %v, want %v", out, payload)
	}
}

func TestTracedReadCloser_DoubleCloseSafe(t *testing.T) {
	trc, exp := newTestTracedReadCloser([]byte("data"))

	if err := trc.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// Second close should not panic or create duplicate spans.
	_ = trc.Close()

	spans := exp.GetSpans()
	downloadSpans := filterSpansByName(spans, "indexd.SDKAdapter.Download.Read")
	if len(downloadSpans) != 1 {
		t.Fatalf("expected 1 Download.Read span after double close, got %d", len(downloadSpans))
	}
}

func TestTracedReadCloser_ReadErrorRecorded(t *testing.T) {
	// If the inner reader returns an error, it should be recorded on the span.
	trc, exp := newTestTracedReadCloser(nil)

	// Replace inner with an error reader.
	trc.inner = &errorReadCloser{err: io.ErrUnexpectedEOF}

	buf := make([]byte, 10)
	_, err := trc.Read(buf)
	if err == nil {
		t.Fatal("expected error from Read")
	}

	trc.Close()

	spans := exp.GetSpans()
	downloadSpans := filterSpansByName(spans, "indexd.SDKAdapter.Download.Read")
	if len(downloadSpans) != 1 {
		t.Fatalf("expected 1 Download.Read span, got %d", len(downloadSpans))
	}
	if downloadSpans[0].Status.Code != codes.Error {
		t.Fatalf("expected span status Error, got %s", downloadSpans[0].Status.Code)
	}
}

// errorReadCloser always returns an error on Read.
type errorReadCloser struct {
	err error
}

func (e *errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e *errorReadCloser) Close() error {
	return nil
}
