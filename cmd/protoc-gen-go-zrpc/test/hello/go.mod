module github.com/crazyfrankie/zrpc/cmd/protoc-gen-go-zrpc/test/hello

go 1.23.5

replace (
	github.com/crazyfrankie/zrpc => ../../../../../zrpc
)
require (
	google.golang.org/protobuf v1.36.5
)
