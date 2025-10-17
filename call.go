package zrpc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crazyfrankie/zrpc/codec"
	"github.com/crazyfrankie/zrpc/mem"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
)

// Invoke sends the RPC request on the wire and returns after response is
// received.  This is typically called by generated code.
func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	// If the user adds middleware, execute the middleware first
	if c.opt.clientMiddleware != nil {
		return c.opt.clientMiddleware(ctx, method, args, reply, c, invoke)
	}

	return invoke(ctx, method, args, reply, c)
}

func invoke(ctx context.Context, method string, args any, reply any, c *Client) error {
	if args == nil || reply == nil {
		return ErrInvalidArgument
	}

	// Start the retry loop
	var lastErr error
	for retry := 0; retry <= c.opt.maxRetries; retry++ {
		if retry > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Indexes retreat and wait
			if retry > 1 && c.opt.retryBackoff > 0 {
				backoff := c.opt.retryBackoff * time.Duration(1<<uint(retry-1))
				if backoff > c.opt.maxRetryBackoff {
					backoff = c.opt.maxRetryBackoff
				}

				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}

		// Get a server using round-robin or user provider selection
		target, err := c.discovery.Get(c.opt.balancerMode)
		if err != nil {
			lastErr = err
			continue
		}

		c.mu.RLock()
		pool, ok := c.pools[target]
		c.mu.RUnlock()

		if !ok {
			c.mu.Lock()
			// Double-check pattern to avoid race condition
			if pool, ok = c.pools[target]; !ok {
				pool = newConnPool(c, target, c.opt.maxPoolSize)
				c.pools[target] = pool
			}
			c.mu.Unlock()
		}

		conn, err := pool.get()
		if err != nil {
			lastErr = err
			continue
		}

		// Ensure that connections are put back into the pool or shut down
		connReleased := false
		defer func() {
			if !connReleased {
				pool.put(conn)
			}
		}()

		call, err := newCall(method, args)
		if err != nil {
			return err
		}

		c.mu.Lock()
		if c.closing || c.shutdown {
			c.mu.Unlock()
			return ErrClientConnClosing
		}
		c.mu.Unlock()

		err = c.sendMsg(ctx, conn, call)
		if err != nil {
			// Connection send failed,
			// close the connection instead of putting it back into the pool
			connReleased = true
			conn.Close()
			lastErr = err
			continue
		}

		err = c.recvMsg(ctx, conn, reply)

		// Mark the connection as released and
		// put it back into the connection pool
		connReleased = true
		pool.put(conn)

		if err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	if lastErr != nil {
		return lastErr
	}

	return ErrMaxRetryExceeded
}

func (c *Client) sendMsg(ctx context.Context, conn *clientConn, call *Call) error {
	req, err := call.prepareMessage(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	if c.closing || c.shutdown {
		c.mu.Unlock()
		return ErrClientConnClosing
	}

	isHeartBeat := req.ServiceName == "" && req.ServiceMethod == ""
	if isHeartBeat {
		req.SetHeartBeat(true)
	}

	seq := atomic.AddUint64(&c.sequence, 1) & math.MaxUint64
	req.SetSeq(seq)
	call.Seq = seq
	c.pending[seq] = call
	c.mu.Unlock()

	msgBuffer := req.Encode()
	defer msgBuffer.Free()

	if conn.conn != nil {
		deadline, ok := ctx.Deadline()
		if ok {
			conn.conn.SetWriteDeadline(deadline)
		}
	}

	_, err = conn.conn.Write(msgBuffer.ReadOnlyData())
	if err != nil {
		var e *net.OpError
		if errors.As(err, &e) {
			if e.Err != nil {
				err = fmt.Errorf("net.OpError: %s", e.Err.Error())
			} else {
				err = errors.New("net.OpError")
			}
		}

		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
	}

	return err
}

func (c *Client) recvMsg(ctx context.Context, conn *clientConn, reply any) error {
	if conn.conn != nil {
		deadline, ok := ctx.Deadline()
		if ok {
			conn.conn.SetReadDeadline(deadline)
		}
	}

	// Create message object to receive response
	res := protocol.NewMessage()
	err := res.Decode(conn.conn, c.opt.maxReceiveMessageSize)
	if err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	seq := res.GetSeq()

	c.mu.Lock()
	call, ok := c.pending[seq]
	if !ok || call == nil {
		c.mu.Unlock()
		return fmt.Errorf("missing sequence %d in client, response mismatch", seq)
	}

	if call.Seq != seq {
		c.mu.Unlock()
		return fmt.Errorf("sequence mismatch: expected %d, got %d", call.Seq, seq)
	}

	delete(c.pending, seq)
	c.mu.Unlock()

	// Safe check for response
	if res.GetMessageType() != protocol.Response || !res.CheckMagicNumber() {
		return errors.New("invalid response message")
	}

	// Check for server error response
	if res.GetMessageStatusType() == protocol.Error {
		if res.Metadata != nil && len(res.Metadata[protocol.ServiceError]) > 0 {
			return errors.New(res.Metadata[protocol.ServiceError][0])
		}
		return errors.New("server error")
	}

	// Handle metadata
	if res.Metadata != nil && res.Metadata.Len() > 0 {
		ctx = context.WithValue(ctx, responseKey{}, res.Metadata)
	}

	// Unmarshal response payload
	err = codec.DefaultCodec.Unmarshal(mem.BufferSlice{mem.SliceBuffer(res.Payload)}, reply)

	return err
}

type Call struct {
	ServiceName   string
	ServiceMethod string
	req           any
	Err           error
	Seq           uint64
	Done          chan *Call
}

func (c *Call) done() {
	if c.Done != nil {
		select {
		case c.Done <- c:
		default:
		}
	}
}

func newCall(method string, args any) (*Call, error) {
	if method != "" && method[0] == '/' {
		method = method[1:]
	}
	pos := strings.LastIndex(method, "/")
	if pos == -1 { // Invalid method name syntax.
		return nil, errors.New("invalid method name syntax")
	}
	svc := method[:pos]
	mtd := method[pos+1:]

	return &Call{
		ServiceName:   svc,
		ServiceMethod: mtd,
		req:           args,
		Done:          make(chan *Call, 1),
	}, nil
}

// prepareMessage create a req message for one call.
func (c *Call) prepareMessage(ctx context.Context) (*protocol.Message, error) {
	req := protocol.NewMessage()
	req.SetMessageType(protocol.Request)
	req.ServiceName = c.ServiceName
	req.ServiceMethod = c.ServiceMethod

	// pb marshal
	payload, err := codec.DefaultCodec.Marshal(c.req)
	if err != nil {
		return nil, err
	}
	defer payload.Free()
	req.Payload = payload.Materialize()

	if len(req.Payload) > 1024 {
		req.SetCompressType(protocol.Gzip)
	}

	// prepare metadata
	if req.Metadata == nil {
		md := metadata.New(map[string]string{
			protocol.UserAgentHeader: protocol.UserAgent,
		})
		userMd, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			md = metadata.Join(md, userMd)
		}
		req.Metadata = md
	} else {
		req.Metadata.Set(protocol.UserAgentHeader, protocol.UserAgent)
	}

	// Add timeout to metadata if context has deadline
	if deadline, ok := ctx.Deadline(); ok {
		timeout := time.Until(deadline)
		if timeout > 0 {
			req.Metadata.Set(protocol.TimeoutHeader, protocol.EncodeTimeout(timeout))
		}
	}

	return req, nil
}

// clear all of pending requests
func (c *Client) cleanupCall(call *Call) {
	if call == nil || call.Seq == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.pending, call.Seq)
}
