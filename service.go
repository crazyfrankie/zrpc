package zrpc

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"
)

// MethodHandler is a function type that processes a unary RPC method call.
type MethodHandler func(srv any, ctx context.Context, dec func(any) error, middleware ServerMiddleware) (any, error)

// MethodDesc represents an RPC service's method specification.
type MethodDesc struct {
	MethodName string
	Handler    MethodHandler
}

// ServiceDesc represents an RPC service's specification.
type ServiceDesc struct {
	ServiceName string
	// The pointer to the service interface. Used to check whether the user
	// provided implementation satisfies the interface requirements.
	HandlerType any
	Methods     []MethodDesc
	Metadata    any
}

type service struct {
	serviceImpl any
	methods     map[string]*MethodDesc
	mdata       any
}

// ServiceRegistrar wraps a single method that supports service registration. It
// enables users to pass concrete types other than grpc.Server to the service
// registration methods exported by the IDL generated code.
type ServiceRegistrar interface {
	// RegisterService registers a service and its implementation to the
	// concrete type implementing this interface.  It may not be called
	// once the server has started serving.
	// desc describes the service and its methods and handlers. impl is the
	// service implementation which is passed to the method handlers.
	RegisterService(desc *ServiceDesc, impl any)
}

// RegisterService registers a service and its implementation to the gRPC
// server. It is called from the IDL generated code. This must be called before
// invoking Serve. If ss is non-nil (for legacy code), its type is checked to
// ensure it implements sd.HandlerType.
func (s *Server) RegisterService(sd *ServiceDesc, srv any) {
	if srv != nil {
		ht := reflect.TypeOf(sd.HandlerType).Elem()
		st := reflect.TypeOf(srv)
		if !st.Implements(ht) {
			zap.L().Fatal(fmt.Sprintf("zrpc: Server.RegisterService found the handler of type %v that does not satisfy %v", st, ht))
		}
	}
	s.register(sd, srv)
}

func (s *Server) register(sd *ServiceDesc, srv any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.serve {
		zap.L().Fatal("zrpc: Server.RegisterService after Server.Serve for ", zap.String("name", sd.ServiceName))
	}
	if _, ok := s.serviceMap.Load(sd.ServiceName); ok {
		zap.L().Fatal("zrpc: Server.RegisterService found duplicate service registration for %q", zap.String("name", sd.ServiceName))
	}

	svc := &service{
		serviceImpl: srv,
		methods:     make(map[string]*MethodDesc),
		mdata:       sd.Metadata,
	}

	for i := range sd.Methods {
		d := &sd.Methods[i]
		svc.methods[d.MethodName] = d
	}
	s.serviceMap.Store(sd.ServiceName, svc)
}
