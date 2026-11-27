package app

import (
	"context"
	"net/http"
)

type traceKey struct{}

type traceInfo struct{ id, header string }

// WithTrace stores the request's W3C trace context.
func WithTrace(ctx context.Context, traceID, traceparent string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceInfo{traceID, traceparent})
}

// TraceID returns the trace id of the request or job, if any.
func TraceID(ctx context.Context) string {
	t, _ := ctx.Value(traceKey{}).(traceInfo)
	return t.id
}

// propagate adds the trace context to an outgoing call, so the receiving
// system can join its logs to ours.
func propagate(ctx context.Context, req *http.Request) {
	if t, ok := ctx.Value(traceKey{}).(traceInfo); ok && t.header != "" {
		req.Header.Set("traceparent", t.header)
	}
}

func traceHeader(ctx context.Context) string {
	t, _ := ctx.Value(traceKey{}).(traceInfo)
	return t.header
}
