package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crazyfrankie/zrpc/server"
	"io"
	"log"
	"net"
	"sync"

	"github.com/crazyfrankie/zrpc/codec"
)

// CallInfo represents an active RPC.
type CallInfo struct {
	Seq           uint64
	ServiceMethod string
	Args          any
	Reply         any
	Err           error
	Done          chan *CallInfo
}

func (c *CallInfo) done() {
	c.Done <- c
}

// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	cc      codec.Codec
	opt     *server.Option
	sending sync.Mutex // protect header
	header  codec.Header
	mu      sync.Mutex // protect seq and pending
	seq     uint64
	pending map[uint64]*CallInfo

	closing  bool
	shutdown bool
}

var _ io.Closer = (*Client)(nil)

var ErrShutDown = errors.New("connection is shutdown")

// Invoke the named function, waits for it to complete,
// and returns its error status.
func (c *Client) Invoke(ctx context.Context, method string, args any, reply any) error {
	call := c.Go(method, args, reply, make(chan *CallInfo, 1))
	select {
	case <-ctx.Done():
		c.deleteCall(c.seq)
		return errors.New("rpc client: call failed:  + ctx.Err().Error()")
	case <-call.Done:
		return call.Err
	}
}

// Go invokes the function asynchronously.
// It returns the Call structure representing the invocation.
func (c *Client) Go(method string, args, reply any, done chan *CallInfo) *CallInfo {
	if done == nil {
		done = make(chan *CallInfo, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}

	call := &CallInfo{
		ServiceMethod: method,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	c.send(call)

	return call
}

// Close the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutDown
	}
	c.closing = true

	return c.cc.Close()
}

// IsAvailable return true if the client does work
func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

// registerCall add the parameter call to client.pending and update client.seq
func (c *Client) registerCall(call *CallInfo) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ErrShutDown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++

	return call.Seq, nil
}

// deleteCall removes the corresponding call from client.pending based on seq and returns the
func (c *Client) deleteCall(seq uint64) *CallInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)

	return call
}

// terminateCalls Called when an error occurs on the server or client side,
// sets shutdown to true and notifies all pending calls of the error message.
func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Err = err
		call.done()
	}
}

func NewClient(conn net.Conn, opt *server.Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with server
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}

	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *server.Option) *Client {
	client := &Client{
		cc:      cc,
		seq:     1, // seq starts with 1, 0 means invalid call
		opt:     opt,
		pending: make(map[uint64]*CallInfo),
	}
	go client.receive()

	return client
}

func parseOptions(opts ...*server.Option) (*server.Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, opts ...*server.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		call := c.deleteCall(h.Seq)
		switch {
		case call == nil:
			// it usually means that Write partially failed
			// and call was already removed.
			err = c.cc.ReadBody(nil)
		case h.Err != "":
			call.Err = fmt.Errorf(h.Err)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Err = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// error occurs, so terminateCalls pending calls
	c.terminateCalls(err)
}

func (c *Client) send(call *CallInfo) {
	// make sure that the client will send a complete request
	c.sending.Lock()
	defer c.sending.Unlock()

	// register this call
	seq, err := c.registerCall(call)
	if err != nil {
		call.Err = err
		call.done()
		return
	}

	// prepare request header
	c.header.Seq = seq
	c.header.ServiceName = call.ServiceMethod
	c.header.Err = ""

	// encode and send the request
	if err := c.cc.Write(&c.header, call.Args); err != nil {
		call := c.deleteCall(seq)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Err = err
			call.done()
		}
	}
}
