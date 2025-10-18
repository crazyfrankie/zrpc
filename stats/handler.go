package stats

import (
	"context"
	"net"
	"time"
)

// Handler defines the interface for RPC stats collection
type Handler interface {
	// HandleRPC processes the RPC stats.
	HandleRPC(ctx context.Context, stats RPCStats)
	// TagRPC can attach some information to the given context.
	// The context used for the rest lifetime of the RPC will be derived from
	// the returned context.
	TagRPC(ctx context.Context, info *RPCTagInfo) context.Context
	// HandleConn processes the Conn stats.
	HandleConn(ctx context.Context, stats ConnStats)
	// TagConn can attach some information to the given context.
	// The returned context will be used for stats handling.
	TagConn(ctx context.Context, info *ConnTagInfo) context.Context
}

// RPCStats contains stats information about RPCs.
type RPCStats interface {
	// IsClient returns true if this RPCStats is from client side.
	IsClient() bool
}

// ConnTagInfo defines the relevant information needed by connection context tagger.
type ConnTagInfo struct {
	// RemoteAddr is the remote address of the corresponding connection.
	RemoteAddr net.Addr
	// LocalAddr is the local address of the corresponding connection.
	LocalAddr net.Addr
}

// RPCTagInfo defines the relevant information needed by RPC context tagger.
type RPCTagInfo struct {
	// FullMethodName is the RPC method in the format of /package.service/method.
	FullMethodName string
	// FailFast indicates if this RPC is failfast.
	FailFast bool
}

// Begin contains stats when an RPC attempt begins.
type Begin struct {
	// Client is true if this Begin is from client side.
	Client bool
	// BeginTime is the time when the RPC attempt begins.
	BeginTime time.Time
	// FailFast indicates if this RPC is failfast.
	FailFast bool
	// IsClientStream indicates whether the RPC is a client streaming RPC.
	IsClientStream bool
	// IsServerStream indicates whether the RPC is a server streaming RPC.
	IsServerStream bool
}

// IsClient indicates if this is from client side.
func (s *Begin) IsClient() bool { return s.Client }

// InPayload contains the information for an incoming payload.
type InPayload struct {
	// Client is true if this InPayload is from client side.
	Client bool
	// Payload is the payload with original type.
	Payload interface{}
	// Data is the serialized message payload.
	Data []byte
	// Length is the length of uncompressed data.
	Length int
	// CompressedLength is the length of compressed data.
	CompressedLength int
	// WireLength is the length of data on wire (compressed, signed, encrypted).
	WireLength int
	// RecvTime is the time when the payload is received.
	RecvTime time.Time
}

// IsClient indicates if this is from client side.
func (s *InPayload) IsClient() bool { return s.Client }

// InHeader contains stats when a header is received.
type InHeader struct {
	// Client is true if this InHeader is from client side.
	Client bool
	// WireLength is the wire length of header.
	WireLength int
	// Header contains the header metadata received.
	Header map[string][]string
	// Compression is the compression algorithm used for the RPC.
	Compression string
	// FullMethod is the full RPC method string, i.e., /package.service/method.
	FullMethod string
	// RemoteAddr is the remote address of the corresponding connection.
	RemoteAddr net.Addr
	// LocalAddr is the local address of the corresponding connection.
	LocalAddr net.Addr
}

// IsClient indicates if this is from client side.
func (s *InHeader) IsClient() bool { return s.Client }

// InTrailer contains stats when a trailer is received.
type InTrailer struct {
	// Client is true if this InTrailer is from client side.
	Client bool
	// WireLength is the wire length of trailer.
	WireLength int
	// Trailer contains the trailer metadata received from the server.
	Trailer map[string][]string
}

// IsClient indicates if this is from client side.
func (s *InTrailer) IsClient() bool { return s.Client }

// OutPayload contains the information for an outgoing payload.
type OutPayload struct {
	// Client is true if this OutPayload is from client side.
	Client bool
	// Payload is the payload with original type.
	Payload interface{}
	// Data is the serialized message payload.
	Data []byte
	// Length is the length of uncompressed data.
	Length int
	// CompressedLength is the length of compressed data.
	CompressedLength int
	// WireLength is the length of data on wire (compressed, signed, encrypted).
	WireLength int
	// SentTime is the time when the payload is sent.
	SentTime time.Time
}

// IsClient indicates if this is from client side.
func (s *OutPayload) IsClient() bool { return s.Client }

// OutHeader contains stats when a header is sent.
type OutHeader struct {
	// Client is true if this OutHeader is from client side.
	Client bool
	// Header contains the header metadata sent.
	Header map[string][]string
	// Compression is the compression algorithm used for the RPC.
	Compression string
	// WireLength is the wire length of header.
	WireLength int
	// FullMethod is the full RPC method string, i.e., /package.service/method.
	FullMethod string
	// RemoteAddr is the remote address of the corresponding connection.
	RemoteAddr net.Addr
	// LocalAddr is the local address of the corresponding connection.
	LocalAddr net.Addr
}

// IsClient indicates if this is from client side.
func (s *OutHeader) IsClient() bool { return s.Client }

// OutTrailer contains stats when a trailer is sent.
type OutTrailer struct {
	// Client is true if this OutTrailer is from client side.
	Client bool
	// Trailer contains the trailer metadata sent to the client.
	Trailer map[string][]string
	// WireLength is the wire length of trailer.
	WireLength int
}

// IsClient indicates if this is from client side.
func (s *OutTrailer) IsClient() bool { return s.Client }

// End contains stats when an RPC ends.
type End struct {
	// Client is true if this End is from client side.
	Client bool
	// BeginTime is the time when the RPC began.
	BeginTime time.Time
	// EndTime is the time when the RPC ends.
	EndTime time.Time
	// Trailer contains the trailer metadata received from the server.
	Trailer map[string][]string
	// Error is the error the RPC ended with. It is an error generated from
	// status.Status and can be converted back to status.Status using
	// status.FromError if non-nil.
	Error error
}

// IsClient indicates if this is from client side.
func (s *End) IsClient() bool { return s.Client }

// ConnStats contains stats information about connections.
type ConnStats interface {
	// IsClient returns true if this ConnStats is from client side.
	IsClient() bool
}

// ConnBegin contains the stats of a connection when it is established.
type ConnBegin struct {
	// Client is true if this ConnBegin is from client side.
	Client bool
}

// IsClient indicates if this is from client side.
func (s *ConnBegin) IsClient() bool { return s.Client }

// ConnEnd contains the stats of a connection when it ends.
type ConnEnd struct {
	// Client is true if this ConnEnd is from client side.
	Client bool
}

// IsClient indicates if this is from client side.
func (s *ConnEnd) IsClient() bool { return s.Client }
