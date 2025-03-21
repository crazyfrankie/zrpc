package zrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/crazyfrankie/zrpc/codec"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
)

func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	// create a call info
	call, err := newCall(method, args)
	if err != nil {
		return err
	}

	// select conn to send message
	conn, err := c.getConn()
	if err != nil {
		return err
	}

	// 发送请求
	if err := c.sendMsg(ctx, conn, call); err != nil {
		return err
	}
	// 接收响应
	err = c.recvMsg(ctx, conn, reply)
	c.removeConn(conn)

	return err
}

func (c *Client) sendMsg(ctx context.Context, conn *clientConn, call *Call) error {
	req, err := call.prepareMessage(ctx)
	if err != nil {
		return err
	}

	c.mu.RLock()
	if c.closing || c.shutdown {
		c.mu.Unlock()
		return ErrClientConnClosing
	}

	isHeartBeat := req.ServiceName == "" && req.ServiceMethod == ""
	if isHeartBeat {
		req.SetHeartBeat(true)
	}
	seq := c.sequence
	c.mu.RUnlock()

	// Increment the sequence number and pending
	c.nextSeq(call)

	req.SetSeq(seq)
	data := req.Encode()
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
	}

	return err
}

func (c *Client) recvMsg(ctx context.Context, conn *clientConn, reply any) error {
	res := protocol.NewMessage()
	err := res.Decode(conn.conn, c.opt.maxReceiveMessageSize)
	if err != nil {
		return err
	}

	seq := res.GetSeq()

	// safe check
	if res.GetMessageType() != protocol.Response || !res.CheckMagicNumber() {
		return errors.New("handle server response error")
	}

	c.mu.Lock()
	delete(c.pending, seq)
	c.mu.Unlock()

	if res.Metadata.Len() != 0 {
		ctx = context.WithValue(ctx, responseKey{}, res.Metadata)
	}

	d := codec.GetBufferSliceFromRequest(res)
	err = codec.DefaultCodec.Unmarshal(d, reply)

	return err
}

type Call struct {
	ServiceName   string
	ServiceMethod string
	req           any
	Err           error
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
