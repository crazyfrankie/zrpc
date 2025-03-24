# ZRPC

zrpc is a rpc framework like [rpcx](https://github.com/smallnest/rpcx) and [gRPC](https://github.com/grpc/grpc-go).

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

# About zRPC
## protocol 
zrpc encapsulates the zrpc protocol based on tcp, the protocol framework like this.
![img.png](image/img.png)

## message payload
Referring to gRPC [protoc-gen-go-grpc](https://github.com/grpc/grpc-go/tree/master/cmd/protoc-gen-go-grpc), the
Custom implementation of [protoc-gen-go-zrpc](https://github.com/crazyfrankie/zrpc/tree/master/cmd/protoc-gen-go-zrpc), for staking code generation.

At the same time there are also considerations to use the reverse proxy approach, that is, in the `Client` to implement the reflection mechanism to dynamically modify the function to avoid code generation, specific ideas in [dynamic-agent](https://github.com/crazyfrankie/dynamic-agent), will be updated in the future.