package peer

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Peer contains the information of the peer for an RPC, such as the address
// and authentication information.
type Peer struct {
	// Addr is the peer address.
	Addr net.Addr
	// LocalAddr is the local address.
	LocalAddr net.Addr
	// AuthInfo is the authentication information of the transport.
	// It is nil if there is no transport security being used.
	//AuthInfo credentials.AuthInfo
}

// String ensures the Peer types implements the Stringer interface in order to
// allow to print a context with a peerKey value effectively.
func (p *Peer) String() string {
	if p == nil {
		return "Peer<nil>"
	}
	sb := &strings.Builder{}
	sb.WriteString("Peer{")
	if p.Addr != nil {
		fmt.Fprintf(sb, "Addr: '%s', ", p.Addr.String())
	} else {
		fmt.Fprintf(sb, "Addr: <nil>, ")
	}
	if p.LocalAddr != nil {
		fmt.Fprintf(sb, "LocalAddr: '%s', ", p.LocalAddr.String())
	} else {
		fmt.Fprintf(sb, "LocalAddr: <nil>, ")
	}
	//if p.AuthInfo != nil {
	//	fmt.Fprintf(sb, "AuthInfo: '%s'", p.AuthInfo.AuthType())
	//} else {
	//	fmt.Fprintf(sb, "AuthInfo: <nil>")
	//}
	sb.WriteString("}")

	return sb.String()
}

type peerKey struct{}

// NewContext creates a new context with peer information attached.
func NewContext(ctx context.Context, p *Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, p)
}

// FromContext returns the peer information in ctx if it exists.
func FromContext(ctx context.Context) (p *Peer, ok bool) {
	p, ok = ctx.Value(peerKey{}).(*Peer)
	return
}
