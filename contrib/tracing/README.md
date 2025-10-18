# zRPC OpenTelemetry Tracing

This package provides OpenTelemetry tracing instrumentation for zRPC framework, similar to the gRPC OpenTelemetry implementation.

## Features

- **Distributed Tracing**: Full support for OpenTelemetry distributed tracing
- **Metrics Collection**: Automatic collection of RPC metrics (duration, message size, etc.)
- **Flexible Filtering**: Support for filtering which RPCs to trace
- **Context Propagation**: Automatic trace context propagation across service boundaries
- **Both Client and Server**: Support for both client-side and server-side instrumentation

## Usage

### Server-side Tracing

```go
package main

import (
    "github.com/crazyfrankie/zrpc"
    "github.com/crazyfrankie/zrpc/contrib/tracing"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    // Initialize tracer
    exp, _ := jaeger.New(jaeger.WithCollectorEndpoint())
    tp := trace.NewTracerProvider(trace.WithBatcher(exp))
    otel.SetTracerProvider(tp)

    // Create server with tracing
    server := zrpc.NewServer(
        zrpc.WithStatsHandler(tracing.NewServerHandler(
            tracing.WithTracerProvider(tp),
            tracing.WithFilter(tracing.HealthCheckFilter()),
            tracing.WithMessageEvents(true),
        )),
    )
    
    // Register your services...
    server.Serve()
}
```

### Client-side Tracing

```go
client, err := zrpc.NewClient(
    context.Background(),
    "localhost:8080",
    zrpc.WithStatsHandler(tracing.NewClientHandler(
        tracing.WithTracerProvider(tp),
    )),
)
```

## Configuration Options

- `WithTracerProvider(tp)`: Use custom tracer provider
- `WithMeterProvider(mp)`: Use custom meter provider  
- `WithFilter(f)`: Filter which RPCs to trace
- `WithMessageEvents(bool)`: Enable/disable message events
- `WithPropagators(p)`: Use custom context propagators

## Filters

Built-in filters:
- `AcceptAll()`: Trace all RPCs
- `RejectAll()`: Trace no RPCs
- `HealthCheckFilter()`: Exclude health check RPCs
- `MethodFilter(methods...)`: Only trace specific methods
- `MethodPrefixFilter(prefixes...)`: Trace methods with specific prefixes

You can combine filters using `Any()`, `All()`, and `Not()`.

## Metrics

The instrumentation automatically collects these metrics:
- `rpc.server.duration` / `rpc.client.duration`: RPC duration
- `rpc.server.request.size` / `rpc.client.request.size`: Request message size
- `rpc.server.response.size` / `rpc.client.response.size`: Response message size
- `rpc.server.requests_per_rpc` / `rpc.client.requests_per_rpc`: Messages per RPC
