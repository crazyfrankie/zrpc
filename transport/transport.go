package transport

// ServerTransport is the common interface for all gRPC server-side transport
// implementations.
//
// Methods may be called concurrently from multiple goroutines, but
// Write methods for a given Stream will be called serially.
//type ServerTransport interface {
//	// HandleStreams receives incoming streams using the given handler.
//	HandleStreams(context.Context, func(*ServerStream))
//
//	// Close tears down the transport. Once it is called, the transport
//	// should not be accessed any more. All the pending streams and their
//	// handlers will be terminated asynchronously.
//	Close(err error)
//
//	// Peer returns the peer of the server transport.
//	Peer() *peer.Peer
//
//	// Drain notifies the client this ServerTransport stops accepting new RPCs.
//	Drain(debugData string)
//}
