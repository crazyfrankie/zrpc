# ZRPC

zrpc is a rpc framework like [rpcx](https://github.com/smallnest/rpcx) and [gRPC](https://github.com/grpc/grpc-go).

zrpc encapsulates the zrpc protocol based on tcp, the protocol framework like this.
![img.png](image/img.png)

# Installation
All you need to do is introduce the following into your code, then run `go mod tidy` and it will automatically fetch the necessary dependencies.
```
import "github.com/crazyfrankie/zrpc"
```

# Examples
You can find all examples at [zrpc/examples](https://github.com/crazyfrankie/zrpc/tree/master/examples)

This is a simple example creating servers and clients with zrpc.

`server`
```
// define your biz server

s := zrpc.NewServer()
example.RegisterExampleServiceServer(s, &ExampleService{})

s.Server("tcp", ":8080")
```
`client`
```
client := zrpc.NewClient(":8080")
ec := example.NewExampleServiceClient(client)

res, err := ec.ExampleCall(ctx, &ExampleCallRequest{})
```
