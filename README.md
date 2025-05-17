# ZRPC

[中文](README_CN.md) ZRPC is a lightweight, high-performance RPC framework inspired by [gRPC](https://github.com/grpc/grpc-go) and [rpcx](https://github.com/smallnest/rpcx). It provides a simple API while maintaining excellent performance and reliability.

## Features

- High-performance TCP long connection communication
- Protocol Buffer serialization support
- Multiple service discovery methods (Memory, ETCD)
- Client-side load balancing
- Connection pool reuse
- Efficient worker pool design
- Heartbeat mechanism
- Middleware support
- TLS security transport
- Comprehensive metrics
- Connection multiplexing (allowing a single TCP connection to handle multiple concurrent requests)

## Architecture Design

### Protocol Design

ZRPC uses a custom binary protocol with the following header design:

```
+-------+--------+----------+------------+---------+
| Magic | Ver    | Msg Type | Comp Type | Seq ID  |
+-------+--------+----------+------------+---------+
|  1B   |   1B   |    1B    |    1B     |   8B   |
+-------+--------+----------+------------+---------+
```

- Magic: Magic number for ZRPC protocol identification
- Version: Protocol version number
- Message Type: Message type (request/response)
- Compress Type: Compression type
- Sequence ID: Request sequence number

### Serialization

Uses Protocol Buffers for serialization by default, with an extensible Codec interface:

```go
type Codec interface {
    Marshal(v interface{}) ([]byte, error)
    Unmarshal(data []byte, v interface{}) error
}
```

## Client Design

### Connection Pool

Implements a two-tier queue connection pool:

1. Hot Queue: 
- Channel-based implementation
- For fast connection acquisition
- Size is half of the total pool size

2. Cold Queue:
- Array-based implementation
- Serves as backup connection pool
- Supports connection expansion

Features:
- Connection prewarming
- Automatic retry
- Idle connection recycling
- Maximum connection limit

### Load Balancing

Supports multiple load balancing strategies:

- Random
- RoundRobin
- Custom extension support

## Server Design

### Worker Pool

Implements an efficient dynamic worker pool with:

1. Adaptive Scaling:
- Dynamic expansion and contraction
- Automatic adjustment based on system load
- Minimum and maximum worker count limits

2. Monitoring Metrics:
- Queue utilization
- Idle worker ratio
- CPU and memory usage
- Request latency statistics

3. Optimization Strategies:
- Quick Scale Up: 20% rapid expansion for high concurrency
- Smooth Contraction: Dynamic adjustment based on load metrics
- Stack Auto-Recovery: Worker reset after 65536 requests to prevent stack growth

### Middleware Support

Complete middleware mechanism:

1. Server Middleware:
```go
type ServerMiddleware func(ctx context.Context, req interface{}, info *ServerInfo, handler Handler) (resp interface{}, err error)
```

2. Client Middleware:
```go
type ClientMiddleware func(ctx context.Context, method string, req, reply interface{}, cc *Client, invoker Invoker) error
```

## Performance Optimizations

1. Memory Optimization:
- Object reuse with sync.Pool
- Two-tier buffer design
- Efficient memory allocation

2. Concurrency Optimization:
- Goroutine pool reuse
- Connection pool management
- Request batching
- Stack auto-recovery

3. Network Optimization:
- Long connection reuse
- Heartbeat mechanism
- Compression support
- Adaptive buffering

## Installation

```go
import "github.com/crazyfrankie/zrpc"
```

Then run:
```bash
go mod tidy
```

## Quick Start

Server:
```go
server := zrpc.NewServer(
    zrpc.WithWorkerPool(100),
    zrpc.WithTLSConfig(tlsConfig),
)
pb.RegisterYourServiceServer(server, &YourService{})
server.Serve("tcp", ":8080")
```

Client:
```go
client := zrpc.NewClient("localhost:8080",
    zrpc.DialWithMaxPoolSize(100),
    zrpc.DialWithHeartbeat(30 * time.Second),
)
defer client.Close()

c := pb.NewYourServiceClient(client)
resp, err := c.YourMethod(ctx, req)
```

## Future Plans

1. Additional service discovery methods
2. Circuit breaking and rate limiting
3. Tracing and monitoring integration