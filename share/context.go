package share

import (
	"context"
	"net"
)

type connKey struct{}

// SetConnection adds the connection to the context to be able to get
// information about the destination ip and port for an incoming RPC. This also
// allows any unary or streaming interceptors to see the connection.
func SetConnection(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, conn)
}

// GetConnection gets the connection from the context.
func GetConnection(ctx context.Context) net.Conn {
	return ctx.Value(connKey{}).(net.Conn)
}
