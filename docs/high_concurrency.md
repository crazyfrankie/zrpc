# Optimizing zRPC for High Concurrency

This document provides guidelines for tuning and configuring zRPC to handle high-concurrency scenarios, particularly when dealing with tens of thousands of concurrent connections.

## Key Improvements in ServerV2

The ServerV2 implementation includes several critical improvements over the original Server implementation:

1. **Function-based worker recycling** - Based on gRPC's approach, this prevents unbounded stack growth in worker goroutines.
2. **Dynamic worker pool sizing** - Automatically scales the number of workers based on load.
3. **Enhanced concurrency safety** - Protects against race conditions and deadlocks.
4. **Panic recovery** - Workers recover from panics to maintain service stability.
5. **Emergency scaling** - Quickly scales up worker count during sudden traffic spikes.

## Configuration Guidelines

### Worker Pool Configuration

```go
server := zrpc.NewServerV2(
    // Start with reasonable min and max worker counts
    zrpc.WithWorkerPoolSize(20, 500),
    
    // Task queue should be sized based on expected request bursts
    zrpc.WithTaskQueueSize(10000),
    
    // How frequently to adjust worker pool size
    zrpc.WithWorkerPoolAdjustInterval(time.Second * 2),
)
```

### Recommended Settings by Concurrency Level

| Concurrency Level | Min Workers | Max Workers | Task Queue Size | Adjustment Interval |
|-------------------|-------------|-------------|-----------------|---------------------|
| < 100             | 5           | 50          | 1000            | 5s                  |
| 100 - 1,000       | 10          | 200         | 5000            | 3s                  |
| 1,000 - 10,000    | 20          | 500         | 10000           | 2s                  |
| > 10,000          | 50          | 1000        | 20000           | 1s                  |

## System-Level Optimizations

For extremely high concurrency (10,000+ connections), also consider these system-level optimizations:

### File Descriptor Limits

Increase the system file descriptor limits:

```sh
# Check current limits
ulimit -n

# Temporary increase
ulimit -n 65535

# Permanent increase (add to /etc/security/limits.conf)
# username soft nofile 65535
# username hard nofile 65535
```

### TCP Connection Tuning

Optimize kernel TCP parameters:

```sh
# Increase local port range
sysctl -w net.ipv4.ip_local_port_range="10000 65535"

# Reuse sockets in TIME_WAIT state
sysctl -w net.ipv4.tcp_tw_reuse=1

# Faster timeout for closed connections
sysctl -w net.ipv4.tcp_fin_timeout=30
```

### Memory Allocation

Consider setting GOGC to a higher value if GC is running too frequently:

```sh
export GOGC=200
```

## Monitoring and Debugging

The ServerV2 implementation includes enhanced metrics for monitoring worker pool performance:

- Worker load distribution
- Queue utilization
- Memory and CPU usage

Use the `/examples/system_check.go` tool to diagnose system-level limitations and get optimization recommendations.

## Common Issues and Solutions

### Problem: Panic in serverWorker at high concurrency

**Symptoms**: Goroutine panics with "chan receive" errors at high concurrency levels.

**Solution**: The improved ServerV2 implementation includes:
- Safer channel operations with proper error handling
- Panic recovery in worker goroutines
- Better coordination during shutdown

### Problem: Performance degradation with 10,000+ connections

**Symptoms**: Response time increases significantly with very high connection counts.

**Solutions**:
1. Increase the task queue size
2. Use batched worker creation during emergency scaling
3. Ensure proper connection cleanup with safe mutex handling

## Performance Testing

Use the included benchmark tool to test performance under various concurrency levels:

```sh
go run examples/high_concurrency_test.go -c 10000 -d 30 -v2=true
```

This will run a 30-second test with 10,000 concurrent connections using ServerV2.
