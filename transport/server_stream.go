package transport

// ServerStream
// Stream 是基于 transport 的抽象
// 参照 gRPC 的设计思路
// Client -> (network) -> transport -> Stream -> Server
// Stream 位于 transport 和 Server 之间, 它主要做的事:
// 1. 负责将 transport 从 http2 DataFrame 中读到的请求数据(请求体)存到缓冲区, 以便 Server 读取以及更进一步的解码
// 2. 获取 Server 调用结果, 将结果写入缓冲区, 将响应数据交给 transport
type ServerStream struct {
}
