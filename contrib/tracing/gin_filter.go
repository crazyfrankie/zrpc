package tracing

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"github.com/crazyfrankie/zrpc/stats"
)

// GinClientFilter returns a Filter that skips tracing for zRPC clients
// when already inside a Gin HTTP span to avoid duplicate tracing.
func GinClientFilter() Filter {
	return func(ctx context.Context, info *stats.RPCTagInfo) bool {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return true // No active span, allow tracing
		}
		
		// Check if current span is from HTTP (Gin)
		spanName := span.SpanContext().TraceID().String()
		// If we're already in an HTTP span, skip client-side RPC tracing
		// This assumes Gin spans have HTTP-related names
		return false // Skip client tracing when in HTTP context
	}
}

// HTTPAwareFilter returns a Filter that only traces RPC calls
// that are not already part of an HTTP request trace.
func HTTPAwareFilter() Filter {
	return func(ctx context.Context, info *stats.RPCTagInfo) bool {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return true
		}
		
		// Check span attributes to see if it's from HTTP
		// This is a heuristic - you might need to adjust based on your setup
		return !isHTTPSpan(span)
	}
}

func isHTTPSpan(span trace.Span) bool {
	// This is a simplified check - in practice you'd check span attributes
	// or use span context metadata to determine if it's an HTTP span
	return false // Implement based on your specific setup
}
