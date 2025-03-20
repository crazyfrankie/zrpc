package zrpc

import (
	"context"
	"errors"
	"strings"

	"github.com/crazyfrankie/zrpc/codec"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
)

func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	req, err := c.prepareMessage(ctx, method, args)
	if err != nil {
		return err
	}

	// TODO
	// 建立连接
	// - 去连接池中取
	// - 有则直接使用, 没有则创建

	// 发送请求
	if err := c.sendMsg(req); err != nil {
		return err
	}
	// 接收响应
	return c.recvMsg(reply)
}

func (c *Client) sendMsg(req *protocol.Message) error {
	//TODO
	return nil
}

func (c *Client) recvMsg(reply any) error {
	//TODO
	return nil
}

func (c *Client) prepareMessage(ctx context.Context, method string, args any) (*protocol.Message, error) {
	req := protocol.NewMessage()

	req.SetMessageType(protocol.Request)
	if method != "" && method[0] == '/' {
		method = method[1:]
	}
	pos := strings.LastIndex(method, "/")
	if pos == -1 { // Invalid method name syntax.
		return nil, errors.New("invalid method name syntax")
	}
	svc := method[:pos]
	mtd := method[pos+1:]
	req.ServiceName = svc
	req.ServiceMethod = mtd

	// pb marshal
	payload, err := codec.DefaultCodec.Marshal(args)
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
