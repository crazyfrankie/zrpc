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
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
)

func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		timeout := 30 * time.Second
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// create a call info
	call, err := newCall(method, args)
	if err != nil {
		return err
	}

	done := make(chan error, 1)

	go func() {
		conn, err := c.pool.get()
		if err != nil {
			done <- err
			return
		}
		defer c.pool.put(conn)

		if err := c.sendMsg(ctx, conn, call); err != nil {
			done <- err
			return
		}

		err = c.recvMsg(ctx, conn, reply)
		done <- err
	}()

	select {
	case <-ctx.Done():
		// Try to clean up pending requests when the context is canceled
		c.cleanupCall(call)
		return ctx.Err()
	case err := <-done:
		return err
	}
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

	data := req.Encode()

	if conn.conn != nil {
		deadline, ok := ctx.Deadline()
		if ok {
			conn.conn.SetWriteDeadline(deadline)
		}
	}

	_, err = conn.conn.Write(*data)
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
	d := codec.GetBufferSliceFromRequest(res)
	defer codec.PutBufferSlice(&d)
	err = codec.DefaultCodec.Unmarshal(d, reply)

	return err
}

type Call struct {
	ServiceName   string
	ServiceMethod string
	req           any
	Err           error
	Seq           uint64
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
	defer codec.PutBufferSlice(&payload)
	req.Payload = payload.ToBytes()
	if len(req.Payload) > 1024 {
		req.SetCompressType(protocol.Gzip)
	}

	// prepare metadata
	if req.Metadata == nil {
		md := metadata.New(map[string]string{
			"user-agent": "zrpc/1.0.0",
		})
		userMd, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			md = metadata.Join(md, userMd)
		}
		req.Metadata = md
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
