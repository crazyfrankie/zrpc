package tracing

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/crazyfrankie/zrpc/stats"
)

type zrpcContextKey struct{}

type zrpcContext struct {
	inMessages  int64
	outMessages int64
	metricAttrs []attribute.KeyValue
	record      bool
}

// serverHandler implements stats.Handler for server-side tracing.
type serverHandler struct {
	*config
	tracer trace.Tracer

	// Metrics
	duration metric.Float64Histogram
	inSize   metric.Int64Histogram
	outSize  metric.Int64Histogram
	inMsg    metric.Int64Histogram
	outMsg   metric.Int64Histogram
}

// NewServerHandler creates a stats.Handler for a zRPC server.
func NewServerHandler(opts ...Option) stats.Handler {
	c := newConfig(opts)
	h := &serverHandler{config: c}

	h.tracer = c.TracerProvider.Tracer(
		ScopeName,
		trace.WithInstrumentationVersion(Version()),
	)

	meter := c.MeterProvider.Meter(
		ScopeName,
		metric.WithInstrumentationVersion(Version()),
	)

	var err error
	if h.duration, err = meter.Float64Histogram(
		"rpc.server.duration",
		metric.WithDescription("Measures the duration of inbound RPC."),
		metric.WithUnit("ms"),
	); err != nil {
		otel.Handle(err)
	}

	if h.inSize, err = meter.Int64Histogram(
		"rpc.server.request.size",
		metric.WithDescription("Measures size of RPC request messages (uncompressed)."),
		metric.WithUnit("By"),
	); err != nil {
		otel.Handle(err)
	}

	if h.outSize, err = meter.Int64Histogram(
		"rpc.server.response.size",
		metric.WithDescription("Measures size of RPC response messages (uncompressed)."),
		metric.WithUnit("By"),
	); err != nil {
		otel.Handle(err)
	}

	if h.inMsg, err = meter.Int64Histogram(
		"rpc.server.requests_per_rpc",
		metric.WithDescription("Measures the number of messages received per RPC."),
		metric.WithUnit("{count}"),
	); err != nil {
		otel.Handle(err)
	}

	if h.outMsg, err = meter.Int64Histogram(
		"rpc.server.responses_per_rpc",
		metric.WithDescription("Measures the number of messages sent per RPC."),
		metric.WithUnit("{count}"),
	); err != nil {
		otel.Handle(err)
	}

	return h
}

// TagRPC can attach some information to the given context.
func (h *serverHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	ctx = Extract(ctx, getIncomingMetadata(ctx), h.Propagators)

	name, attrs := parseFullMethod(info.FullMethodName)
	attrs = append(attrs, semconv.RPCSystemKey.String("zrpc"))

	record := true
	if h.Filter != nil {
		record = h.Filter(ctx, info)
	}

	if record {
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		}
		ctx, _ = h.tracer.Start(
			ctx,
			name,
			opts...,
		)
	}

	gctx := &zrpcContext{
		metricAttrs: attrs,
		record:      record,
	}

	return context.WithValue(ctx, zrpcContextKey{}, gctx)
}

// HandleRPC processes the RPC stats.
func (h *serverHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	h.handleRPC(ctx, rs)
}

func (h *serverHandler) handleRPC(ctx context.Context, rs stats.RPCStats) {
	gctx, _ := ctx.Value(zrpcContextKey{}).(*zrpcContext)
	if gctx != nil && !gctx.record {
		return
	}

	span := trace.SpanFromContext(ctx)
	var messageId int64

	switch rs := rs.(type) {
	case *stats.Begin:
		// RPC开始
	case *stats.InPayload:
		if gctx != nil {
			messageId = atomic.AddInt64(&gctx.inMessages, 1)
			h.inSize.Record(ctx, int64(rs.Length), metric.WithAttributes(gctx.metricAttrs...))
		}

		if h.ReceivedEvent && span.IsRecording() {
			span.AddEvent("message",
				trace.WithAttributes(
					semconv.RPCMessageTypeReceived,
					semconv.RPCMessageIDKey.Int64(messageId),
					semconv.RPCMessageUncompressedSizeKey.Int(rs.Length),
				),
			)
		}
	case *stats.OutPayload:
		if gctx != nil {
			messageId = atomic.AddInt64(&gctx.outMessages, 1)
			h.outSize.Record(ctx, int64(rs.Length), metric.WithAttributes(gctx.metricAttrs...))
		}

		if h.SentEvent && span.IsRecording() {
			span.AddEvent("message",
				trace.WithAttributes(
					semconv.RPCMessageTypeSent,
					semconv.RPCMessageIDKey.Int64(messageId),
					semconv.RPCMessageUncompressedSizeKey.Int(rs.Length),
				),
			)
		}
	case *stats.OutHeader:
		// Header发送
	case *stats.OutTrailer:
		// Trailer发送
	case *stats.End:
		if gctx != nil {
			h.inMsg.Record(ctx, gctx.inMessages, metric.WithAttributes(gctx.metricAttrs...))
			h.outMsg.Record(ctx, gctx.outMessages, metric.WithAttributes(gctx.metricAttrs...))

			elapsedTime := rs.EndTime.Sub(rs.BeginTime)
			h.duration.Record(ctx, float64(elapsedTime)/float64(time.Millisecond), metric.WithAttributes(gctx.metricAttrs...))
		}

		if span.IsRecording() {
			if rs.Error != nil {
				span.RecordError(rs.Error)
				span.SetStatus(codes.Error, rs.Error.Error())
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}
		span.End()
	}
}

// Helper functions
func parseFullMethod(fullMethod string) (string, []attribute.KeyValue) {
	name := fullMethod
	var attrs []attribute.KeyValue
	if name != "" && name[0] == '/' {
		name = name[1:]
	}
	pos := strings.LastIndex(name, "/")
	if pos >= 0 {
		service, method := name[:pos], name[pos+1:]
		attrs = []attribute.KeyValue{
			semconv.RPCServiceKey.String(service),
			semconv.RPCMethodKey.String(method),
		}
	}
	return name, attrs
}

func extractMethod(ctx context.Context) string {
	// Extract method from context - implementation depends on your framework
	return "unknown"
}

func extractService(ctx context.Context) string {
	// Extract service from context - implementation depends on your framework
	return "unknown"
}

func getIncomingMetadata(ctx context.Context) map[string][]string {
	// Extract metadata from context - implementation depends on your framework
	return make(map[string][]string)
}
