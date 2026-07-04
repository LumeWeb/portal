package core

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// traceParentKey is the carrier key for W3C Trace Context headers.
// This matches the standard "traceparent" header name used in HTTP and
// other transport protocols, ensuring compatibility with W3C-compliant
// tracing backends like Tempo, Jaeger, and Zipkin.
const traceParentKey = "traceparent"

// traceContextCarrier is a propagation.TextMapCarrier that carries the
// traceparent header in a single string field. It is used to serialize
// and deserialize trace context across async boundaries (e.g., workflow
// steps dispatched via cron, or across nodes in a horizontally-scaled
// deployment).
type traceContextCarrier struct {
	headers map[string]string
}

func (c *traceContextCarrier) Get(key string) string {
	return c.headers[key]
}

func (c *traceContextCarrier) Set(key, value string) {
	c.headers[key] = value
}

// Keys returns the set of keys present in the carrier, satisfying the
// propagation.TextMapCarrier interface.
func (c *traceContextCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}

// MarshalTraceParent extracts the trace context from ctx and serializes
// it as a W3C traceparent string. Returns an empty string if no span
// context is present in ctx.
//
// Use this to persist trace context across async boundaries (DB, message
// queue, etc.). Restore it with ContextWithTraceParent.
func MarshalTraceParent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}

	carrier := &traceContextCarrier{headers: make(map[string]string)}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get(traceParentKey)
}

// ContextWithTraceParent creates a new context with the trace context
// restored from a W3C traceparent string. If the string is empty or
// invalid, the original context is returned unchanged.
//
// The returned context's spans will be children of the original trace,
// maintaining parent-child trace continuity. For linking independent
// traces (e.g., workflow steps), use SpanLinksFromTraceParents instead.
func ContextWithTraceParent(ctx context.Context, traceParent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	traceParent = strings.TrimSpace(traceParent)
	if traceParent == "" {
		return ctx
	}

	carrier := &traceContextCarrier{headers: map[string]string{traceParentKey: traceParent}}
	return propagation.TraceContext{}.Extract(ctx, carrier)
}

// ParseTraceParent parses a W3C traceparent string and returns the
// embedded SpanContext. Returns false if the string is empty, malformed,
// or contains an invalid span context.
func ParseTraceParent(traceParent string) (trace.SpanContext, bool) {
	traceParent = strings.TrimSpace(traceParent)
	if traceParent == "" {
		return trace.SpanContext{}, false
	}

	carrier := &traceContextCarrier{headers: map[string]string{traceParentKey: traceParent}}
	ctx := propagation.TraceContext{}.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)
	return sc, sc.IsValid()
}

// SpanLinksFromTraceParents parses one or more traceparent strings and
// returns them as trace.Link slice suitable for passing to WithLinks.
// Empty strings are silently skipped. Returns nil if no valid trace
// parents are provided.
//
// Use this to link independent traces (e.g., a workflow step's trace
// linking back to the root workflow trace) without forcing them into a
// single parent-child trace tree.
func SpanLinksFromTraceParents(traceParents ...string) []trace.Link {
	var links []trace.Link
	for _, tp := range traceParents {
		sc, ok := ParseTraceParent(tp)
		if !ok {
			continue
		}
		links = append(links, trace.Link{SpanContext: sc})
	}
	return links
}

// HasTraceParent returns true if the traceparent string is non-empty
// and appears to contain a valid W3C trace context.
func HasTraceParent(traceParent string) bool {
	_, ok := ParseTraceParent(traceParent)
	return ok
}

// Workflow span attribute keys. These are set on every span within a
// workflow step, enabling query-based grouping in Tempo/LogQL:
//
//	workflow.id="42"       — all spans for workflow request 42
//	workflow.name="upload"  — all upload workflow spans
//	workflow.user_id="7"    — all workflows for user 7
//	workflow.hash="..."     — all operations on a specific storage hash
const (
	AttrWorkflowID     = attribute.Key("workflow.id")
	AttrWorkflowName   = attribute.Key("workflow.name")
	AttrWorkflowUserID = attribute.Key("workflow.user_id")
	AttrWorkflowHash   = attribute.Key("workflow.hash")
)

// WorkflowSpanAttributes returns OTEL attributes for workflow span annotation.
// These attributes enable querying spans by workflow ID, name, user, or hash
// across all traces in the workflow, even when steps execute on different nodes.
//
// userID may be nil for anonymous/system operations. hash may be empty.
func WorkflowSpanAttributes(workflowID, workflowName string, userID *uint, hash string) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	if workflowID != "" {
		attrs = append(attrs, AttrWorkflowID.String(workflowID))
	}
	if workflowName != "" {
		attrs = append(attrs, AttrWorkflowName.String(workflowName))
	}
	if userID != nil {
		attrs = append(attrs, AttrWorkflowUserID.Int64(int64(*userID)))
	}
	if hash != "" {
		attrs = append(attrs, AttrWorkflowHash.String(hash))
	}
	return attrs
}
