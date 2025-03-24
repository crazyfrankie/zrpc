package reflection

import "github.com/crazyfrankie/zrpc"

type ZRPCServer interface {
	zrpc.ServiceRegistrar
}

var _ ZRPCServer = (*zrpc.Server)(nil)

func Register(s ZRPCServer) {

}

type ServiceInfoProvider interface {
	GetServiceInfo() map[string]zrpc.ServiceInfo
}
