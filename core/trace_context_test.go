package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupTestTracerProvider creates a real SDK tracer provider with an
// in-memory exporter for testing, and returns a cleanup function.
func setupTestTracerProvider() (trace.TracerProvider, *tracetest.InMemoryExporter, func()) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	cleanup := func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	}
	return tp, exporter, cleanup
}

// TestMarshalTraceParent_NoSpanContext tests that MarshalTraceParent returns
// an empty string when no span context is present in the context.
func TestMarshalTraceParent_NoSpanContext(t *testing.T) {
	tp := MarshalTraceParent(context.Background())
	assert.Empty(t, tp, "should return empty string when no span context")
}

// TestMarshalTraceParent_WithSpanContext tests that MarshalTraceParent
// produces a valid W3C traceparent when a span context is present.
func TestMarshalTraceParent_WithSpanContext(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	traceParent := MarshalTraceParent(ctx)
	assert.NotEmpty(t, traceParent, "should return non-empty traceparent")
	// W3C traceparent format: 00-<32 hex traceID>-<16 hex spanID>-<2 hex flags>
	assert.Len(t, traceParent, 55, "traceparent should be 55 chars")
	assert.Equal(t, "00", traceParent[:2], "version should be 00")
}

// TestContextWithTraceParent_Empty tests that an empty traceparent
// returns the original context unchanged.
func TestContextWithTraceParent_Empty(t *testing.T) {
	ctx := context.Background()
	result := ContextWithTraceParent(ctx, "")
	assert.NotNil(t, result)
}

// TestContextWithTraceParent_Whitespace tests that whitespace-only
// traceparent is treated as empty.
func TestContextWithTraceParent_Whitespace(t *testing.T) {
	ctx := context.Background()
	result := ContextWithTraceParent(ctx, "   ")
	assert.NotNil(t, result)
}

// TestContextWithTraceParent_RoundTrip tests that a traceparent
// marshaled from a context can be restored and produces a child
// span context with the same trace ID.
func TestContextWithTraceParent_RoundTrip(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")

	// Create a parent span and extract its traceparent
	parentCtx, parentSpan := tracer.Start(context.Background(), "parent-span")
	parentSpan.End()

	traceParent := MarshalTraceParent(parentCtx)
	assert.NotEmpty(t, traceParent)

	// Restore trace context into a fresh background context
	restoredCtx := ContextWithTraceParent(context.Background(), traceParent)

	// Create a child span in the restored context
	_, childSpan := tracer.Start(restoredCtx, "child-span")
	childSpan.End()

	// Verify the child span has the same trace ID as the parent
	parentSC := parentSpan.SpanContext()
	childSC := childSpan.SpanContext()

	assert.True(t, parentSC.TraceID().IsValid(), "parent trace ID should be valid")
	assert.True(t, childSC.TraceID().IsValid(), "child trace ID should be valid")
	assert.Equal(t, parentSC.TraceID(), childSC.TraceID(),
		"child span should have same trace ID as parent")
	// The child span should be recorded as a child of the parent
	assert.NotEqual(t, parentSC.SpanID(), childSC.SpanID(),
		"child span should have different span ID than parent")
}

// TestContextWithTraceParent_InvalidString tests that an invalid
// traceparent string is handled gracefully (no panic).
func TestContextWithTraceParent_InvalidString(t *testing.T) {
	ctx := context.Background()
	result := ContextWithTraceParent(ctx, "invalid")
	assert.NotNil(t, result)
}

// TestHasTraceParent tests the HasTraceParent helper.
func TestHasTraceParent(t *testing.T) {
	assert.False(t, HasTraceParent(""))
	assert.False(t, HasTraceParent("  "))
	assert.False(t, HasTraceParent("00-abc-def-01"), "short/invalid traceparent should be false")
}

// TestMarshalTraceParent_NilContext tests nil context handling.
func TestMarshalTraceParent_NilContext(t *testing.T) {
	tp := MarshalTraceParent(nil)
	assert.Empty(t, tp)
}

// TestContextWithTraceParent_NilContext tests nil context handling.
func TestContextWithTraceParent_NilContext(t *testing.T) {
	result := ContextWithTraceParent(nil, "00-abc-def-01")
	assert.NotNil(t, result)
}

// TestRoundTrip_PreservesTraceFlags tests that trace flags (like sampled)
// are preserved through the round-trip.
func TestRoundTrip_PreservesTraceFlags(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")

	// Create a parent span (sampled by default)
	parentCtx, parentSpan := tracer.Start(context.Background(), "parent-span")
	parentSpan.End()

	parentSC := parentSpan.SpanContext()
	traceParent := MarshalTraceParent(parentCtx)

	// Restore and create child
	restoredCtx := ContextWithTraceParent(context.Background(), traceParent)
	_, childSpan := tracer.Start(restoredCtx, "child-span")
	childSpan.End()

	childSC := childSpan.SpanContext()

	// Trace flags should match
	assert.Equal(t, parentSC.TraceFlags(), childSC.TraceFlags(),
		"trace flags should be preserved through round-trip")
}

// TestContextWithTraceParent_CreatesValidSpanContext tests that
// restoring a valid traceparent produces a valid span context.
func TestContextWithTraceParent_CreatesValidSpanContext(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")

	// Create parent and extract traceparent
	parentCtx, parentSpan := tracer.Start(context.Background(), "parent")
	traceParent := MarshalTraceParent(parentCtx)
	parentSpan.End()

	// Restore into fresh context
	restoredCtx := ContextWithTraceParent(context.Background(), traceParent)

	// The restored context should have a valid span context
	restoredSC := trace.SpanContextFromContext(restoredCtx)
	assert.True(t, restoredSC.IsValid(), "restored span context should be valid")
	assert.True(t, restoredSC.IsRemote(), "restored span context should be remote")
}

// --- ParseTraceParent tests ---

// TestParseTraceParent_ValidRoundTrip tests that ParseTraceParent correctly
// parses a traceparent produced by MarshalTraceParent.
func TestParseTraceParent_ValidRoundTrip(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	span.End()

	traceParent := MarshalTraceParent(ctx)
	assert.NotEmpty(t, traceParent)

	sc, ok := ParseTraceParent(traceParent)
	assert.True(t, ok, "should parse valid traceparent")
	assert.True(t, sc.IsValid(), "parsed span context should be valid")
	assert.True(t, sc.IsRemote(), "parsed span context should be remote")
}

// TestParseTraceParent_Empty tests empty string handling.
func TestParseTraceParent_Empty(t *testing.T) {
	_, ok := ParseTraceParent("")
	assert.False(t, ok, "empty string should not parse")
}

// TestParseTraceParent_Whitespace tests whitespace-only string.
func TestParseTraceParent_Whitespace(t *testing.T) {
	_, ok := ParseTraceParent("   ")
	assert.False(t, ok, "whitespace should not parse")
}

// TestParseTraceParent_Invalid tests malformed traceparent.
func TestParseTraceParent_Invalid(t *testing.T) {
	_, ok := ParseTraceParent("not-a-traceparent")
	assert.False(t, ok, "malformed string should not parse")
}

// --- SpanLinksFromTraceParents tests ---

// TestSpanLinksFromTraceParents_Valid tests that valid traceparents
// produce trace.Link entries.
func TestSpanLinksFromTraceParents_Valid(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "root-span")
	span.End()

	traceParent := MarshalTraceParent(ctx)
	assert.NotEmpty(t, traceParent)

	links := SpanLinksFromTraceParents(traceParent)
	assert.Len(t, links, 1, "should produce one link")
	assert.True(t, links[0].SpanContext.IsValid(), "link span context should be valid")
}

// TestSpanLinksFromTraceParents_Multiple tests multiple traceparents.
func TestSpanLinksFromTraceParents_Multiple(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")

	_, span1 := tracer.Start(context.Background(), "span1")
	span1.End()
	tp1 := MarshalTraceParent(trace.ContextWithSpanContext(context.Background(), span1.SpanContext()))

	_, span2 := tracer.Start(context.Background(), "span2")
	span2.End()
	tp2 := MarshalTraceParent(trace.ContextWithSpanContext(context.Background(), span2.SpanContext()))

	links := SpanLinksFromTraceParents(tp1, tp2)
	assert.Len(t, links, 2, "should produce two links")
}

// TestSpanLinksFromTraceParents_SkipsEmpty tests that empty strings
// are silently skipped.
func TestSpanLinksFromTraceParents_SkipsEmpty(t *testing.T) {
	tp, _, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "root")
	span.End()
	validTP := MarshalTraceParent(ctx)

	links := SpanLinksFromTraceParents(validTP, "", "  ", "invalid")
	assert.Len(t, links, 1, "should only produce one link from the valid traceparent")
}

// TestSpanLinksFromTraceParents_AllEmpty tests no valid input.
func TestSpanLinksFromTraceParents_AllEmpty(t *testing.T) {
	links := SpanLinksFromTraceParents("", "  ", "invalid")
	assert.Nil(t, links, "should return nil when no valid traceparents")
}

// --- WithLinks / StartSpan integration tests ---

// TestStartSpan_WithLinks tests that links passed via WithLinks are
// recorded on the span.
func TestStartSpan_WithLinks(t *testing.T) {
	tp, exporter, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")

	// Create a root span to link to
	_, rootSpan := tracer.Start(context.Background(), "root")
	rootSpan.End()
	rootTP := MarshalTraceParent(trace.ContextWithSpanContext(context.Background(), rootSpan.SpanContext()))
	links := SpanLinksFromTraceParents(rootTP)

	// Create a new span with links — no parent-child relationship, just links
	_, linkedSpan := StartSpan(context.Background(), "linked",
		WithLinks(links...),
	)
	linkedSpan.End()

	// Verify the span was exported
	spans := exporter.GetSpans()
	assert.Len(t, spans, 2, "should have exported root + linked span")

	// The linked span should have the link
	linkedSpanData := spans[1]
	assert.NotEmpty(t, linkedSpanData.Links, "linked span should have trace links")
	assert.Equal(t, rootSpan.SpanContext().TraceID(), linkedSpanData.Links[0].SpanContext.TraceID(),
		"link should reference root span's trace ID")
}

// TestStartSpan_WithLinks_Empty tests that no links are set when none provided.
func TestStartSpan_WithLinks_Empty(t *testing.T) {
	tp, exporter, cleanup := setupTestTracerProvider()
	defer cleanup()

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "test")
	span.End()

	_, linkedSpan := StartSpan(context.Background(), "no-links")
	linkedSpan.End()

	spans := exporter.GetSpans()
	assert.Len(t, spans, 2)
	assert.Empty(t, spans[1].Links, "span should have no links")
}

// --- WorkflowSpanAttributes tests ---

// TestWorkflowSpanAttributes_AllFields tests attribute generation with all fields.
func TestWorkflowSpanAttributes_AllFields(t *testing.T) {
	userID := uint(42)
	attrs := WorkflowSpanAttributes("req-1", "upload", &userID, "QmHash123")

	assert.Len(t, attrs, 4, "should produce 4 attributes")
	// Verify keys
	keys := make(map[string]bool, len(attrs))
	for _, a := range attrs {
		keys[string(a.Key)] = true
	}
	assert.True(t, keys["workflow.id"])
	assert.True(t, keys["workflow.name"])
	assert.True(t, keys["workflow.user_id"])
	assert.True(t, keys["workflow.hash"])
}

// TestWorkflowSpanAttributes_NilUser tests nil user ID is omitted.
func TestWorkflowSpanAttributes_NilUser(t *testing.T) {
	attrs := WorkflowSpanAttributes("req-1", "upload", nil, "QmHash")

	assert.Len(t, attrs, 3, "should produce 3 attributes (no user_id)")
	for _, a := range attrs {
		assert.NotEqual(t, "workflow.user_id", string(a.Key), "should not include user_id")
	}
}

// TestWorkflowSpanAttributes_EmptyHash tests empty hash is omitted.
func TestWorkflowSpanAttributes_EmptyHash(t *testing.T) {
	userID := uint(1)
	attrs := WorkflowSpanAttributes("req-1", "upload", &userID, "")

	assert.Len(t, attrs, 3, "should produce 3 attributes (no hash)")
	for _, a := range attrs {
		assert.NotEqual(t, "workflow.hash", string(a.Key), "should not include hash")
	}
}

// TestWorkflowSpanAttributes_AllEmpty tests minimal attributes.
func TestWorkflowSpanAttributes_AllEmpty(t *testing.T) {
	attrs := WorkflowSpanAttributes("", "", nil, "")

	// Should still produce id and name (even if empty), since the condition
	// checks if either is non-empty — but both empty means no attrs.
	// Actually the condition is OR, so if both are empty, no attrs are added.
	assert.Empty(t, attrs, "should produce no attributes when both id and name are empty")
}
