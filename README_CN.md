# ZRPC

ZRPC 是一个轻量级、高性能的 RPC 框架，借鉴了 gRPC 和 rpcx 的优秀设计。它提供了简单易用的 API，同时在性能和可靠性方面做了诸多优化。

## 特性

- 基于 TCP 长连接的高性能通信
- Protocol Buffer 序列化支持
- 支持多种服务发现方式 (Memory, ETCD)
- 客户端负载均衡
- 连接池复用
- 高效的工作池设计
- 心跳保活机制
- 支持中间件扩展
- 支持 TLS 安全传输
- 完整的度量指标

## 架构设计

### 协议设计

ZRPC 使用自定义的二进制协议，协议头设计如下：

```
+-------+--------+----------+------------+---------+
| Magic | Ver    | Msg Type | Comp Type | Seq ID  |
+-------+--------+----------+------------+---------+
|  1B   |   1B   |    1B    |    1B     |   8B   |
+-------+--------+----------+------------+---------+
```

- Magic：魔数，用于识别 ZRPC 协议
- Version：协议版本号
- Message Type：消息类型（请求/响应）
- Compress Type：压缩类型
- Sequence ID：请求序列号

### 序列化机制

默认使用 Protocol Buffers 进行序列化，同时提供了可扩展的 Codec 接口：

```go
type Codec interface {
    Marshal(v interface{}) ([]byte, error)
    Unmarshal(data []byte, v interface{}) error
}
```

## 客户端设计

### 连接池

实现了两级队列的连接池设计：

1. 热队列（Hot Queue）：
- 基于 channel 实现
- 用于快速获取连接
- 大小为总连接池的一半

2. 冷队列（Cold Queue）：
- 基于数组实现
- 作为备用连接池
- 支持连接扩容

特性：
- 支持连接预热
- 自动失败重试
- 空闲连接回收
- 支持最大连接数限制

### 负载均衡

支持多种负载均衡策略：

- Random：随机选择
- RoundRobin：轮询
- 支持自定义扩展

## 服务端设计

### 工作池（Worker Pool）

实现了高效的动态工作池，具有以下特性：

1. 自适应伸缩：
- 支持动态扩缩容
- 基于系统负载自动调整
- 有最小和最大工作者数量限制

2. 监控指标：
- 队列使用率
- 空闲工作者比例
- CPU 和内存使用率
- 请求延迟统计

3. 优化策略：
- 紧急扩容（Quick Scale Up）：遇到高并发时快速扩容 20%
- 平滑收缩：基于负载指标动态调整
- 堆栈自动回收：每处理 65536 个请求后自动重置 Worker，避免堆栈持续增长

### 中间件支持

提供了完整的中间件机制：

1. 服务端中间件：
```go
type ServerMiddleware func(ctx context.Context, req interface{}, info *ServerInfo, handler Handler) (resp interface{}, err error)
```

2. 客户端中间件：
```go
type ClientMiddleware func(ctx context.Context, method string, req, reply interface{}, cc *Client, invoker Invoker) error
```

## 性能优化

1. 内存优化：
- 使用 sync.Pool 复用对象
- 两级 buffer 设计
- 高效的内存分配策略

2. 并发优化：
- goroutine 池复用
- 连接池管理
- 请求批处理
- 堆栈自动回收

3. 网络优化：
- 长连接复用
- 心跳保活
- 压缩支持
- 自适应缓冲区

## 安装

```go
import "github.com/crazyfrankie/zrpc"
```

然后运行：
```bash
go mod tidy
```

## 快速开始

服务端：
```go
server := zrpc.NewServer(
    zrpc.WithWorkerPool(100),
    zrpc.WithTLSConfig(tlsConfig),
)
pb.RegisterYourServiceServer(server, &YourService{})
server.Serve("tcp", ":8080")
```

客户端：
```go
client := zrpc.NewClient("localhost:8080",
    zrpc.DialWithMaxPoolSize(100),
    zrpc.DialWithHeartbeat(30 * time.Second),
)
defer client.Close()

c := pb.NewYourServiceClient(client)
resp, err := c.YourMethod(ctx, req)
```

## 未来规划

1. 增加更多服务发现方式支持
2. 添加熔断和限流功能
3. 支持跟踪和监控集成
4. 优化协议编码效率
5. 增加更多压缩算法支持
