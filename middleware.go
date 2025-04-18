package zrpc

import "context"

// Invoker is called by ClientMiddleware to complete RPCs.
type Invoker func(ctx context.Context, method string, req, reply any, cc *Client) error

// ClientMiddleware intercepts one-dimensional RPCs performed by the client.
// When creating a ClientOption, middleware can be passed in using DialWithMiddleware() or DialWithChainMiddleware().
// WithUnaryInterceptor() or WithChainUnaryInterceptor() specified as a DialOption.
// When a monadic interceptor is set on the Client, zRPC
// delegates all unary RPC calls to the interceptor, and it is the interceptor's responsibility to // delegate all unary RPC calls to the Client.
// It is the responsibility of the interceptor to invoke the invoker to complete the RPC processing.
// The req and reply are the corresponding request and response messages,
// cc is the Client that invokes the RPC. Invoker is the handler that completes the RPC and is invoked by the interceptor.
type ClientMiddleware func(ctx context.Context, method string, req, reply any, cc *Client, invoker Invoker) error

// ServerInfo consists of various information about an RPC on the server side.
// The middleware may mutate all per-rpc information.
type ServerInfo struct {
	// Server is the service implementation the user provides. This is read-only.
	Server any
	// FullMethod is the full RPC method string, i.e., /package.service/method.
	FullMethod string
}

// Handler defines the handler invoked by UnaryServerInterceptor to complete the normal
// execution of a unary RPC.
type Handler func(ctx context.Context, req any) (any, error)

// ServerMiddleware provides a hook to intercept the execution of an RPC on the server,
// info contains all the information of this RPC the middleware can operate on.
// And handler is the wrapper of the service method implementation.
// It is the responsibility of the middleware to invoke handler to complete the RPC.
type ServerMiddleware func(ctx context.Context, req any, info *ServerInfo, handler Handler) (resp any, err error)
