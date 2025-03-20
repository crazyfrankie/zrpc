package zrpc

import "context"

type ClientInterface interface {
	Invoke(ctx context.Context, method string, args any, reply any) error
}

// Assert *ClientConn implements ClientConnInterface.
var _ ClientInterface = (*Client)(nil)

type Client struct {
}

func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	return nil
}
