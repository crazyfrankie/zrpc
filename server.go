package zrpc

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/crazyfrankie/zrpc/codec"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
	"github.com/crazyfrankie/zrpc/share"
)

const (
	ReadBufferSize = 1024
)

// Server represents an RPC Server.
type Server struct {
	lis        net.Listener
	opt        *serverOption
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
	serviceMap sync.Map       // service name -> service info
	serveWG    sync.WaitGroup // counts active Serve goroutines for Stop/GracefulStop

	cv             *sync.Cond
	serve          bool
	handleMsgCount int32
	inShutdown     int32
	done           chan struct{}
}

// NewServer returns a new rpc server
func NewServer(opts ...ServerOption) *Server {
	opt := defaultServerOption
	for _, o := range opts {
		o(opt)
	}

	s := &Server{
		opt:   opt,
		conns: make(map[net.Conn]struct{}),
		done:  make(chan struct{}),
	}
	s.cv = sync.NewCond(&s.mu)

	return s
}

// Serve starts and listens RPC requests.
// It is blocked until receiving connections from clients.
func (s *Server) Serve(network, address string) error {
	lis, err := s.makeListener(network, address)
	if err != nil {
		return err
	}

	return s.serveListener(lis)
}

// serveListener accepts incoming connections on the Listener lis,
// creating a new service goroutine for each.
// The service goroutines read requests and then call services to reply to them.
func (s *Server) serveListener(lis net.Listener) error {
	var tempDelay time.Duration // how long to sleep on accept failure

	s.mu.Lock()
	s.lis = lis
	s.serve = true
	s.mu.Unlock()

	for {
		conn, err := lis.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && (ne.Timeout() || isRecoverableError(err)) {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				s.mu.Lock()
				fmt.Printf("Accept error: %v; retrying in %v", err, tempDelay)
				s.mu.Unlock()
				timer := time.NewTimer(tempDelay)
				select {
				case <-timer.C:
				case <-s.done:
					timer.Stop()
					return nil
				}
				continue
			}
		}
		tempDelay = 0

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.serveWG.Add(1)
		go func() {
			s.serveConn(context.Background(), conn)
			s.serveWG.Done()
		}()
	}
}

// serveConn runs the server on a single connection.
// serveConn blocks, serving the connection until the client hangs up.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	ctx = share.SetConnection(ctx, conn)
	if s.isShutDown() {
		s.removeConn(conn)
		return
	}

	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			ss := runtime.Stack(buf, false)
			if ss > size {
				ss = size
			}
			buf = buf[:ss]
			zap.L().Error(fmt.Sprintf("serving %s panic error: %s, stack:\n %s", conn.RemoteAddr(), err, buf))
		}

		// make sure all inflight requests are handled and all drained
		if s.isShutDown() {
			<-s.done
		}

		s.removeConn(conn)
	}()

	if tlsConn, ok := conn.(*tls.Conn); ok {
		if d := s.opt.readTimeout; d != 0 {
			conn.SetReadDeadline(time.Now().Add(d))
		}
		if d := s.opt.writeTimeout; d != 0 {
			conn.SetWriteDeadline(time.Now().Add(d))
		}
		if err := tlsConn.Handshake(); err != nil {
			zap.L().Error(fmt.Sprintf("rpcx: TLS handshake error from %s: %v", conn.RemoteAddr(), err))
			return
		}
	}

	r := bufio.NewReaderSize(conn, ReadBufferSize)

	// read requests and handle it
	for {
		if s.isShutDown() {
			return
		}

		now := time.Now()
		if s.opt.readTimeout != 0 {
			conn.SetReadDeadline(now.Add(s.opt.readTimeout))
		}

		req, err := s.readRequest(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				zap.L().Info("client has closed the connection:", zap.String("addr", conn.RemoteAddr().String()))
			} else if errors.Is(err, net.ErrClosed) {
				zap.L().Info("zrpc: connection is closed:", zap.String("addr", conn.RemoteAddr().String()))
			} else { // wrong data
				zap.L().Warn("rpcx: failed to read request: ", zap.String("err", err.Error()))
			}

			return
		}

		// inject metadata to context
		ctx = metadata.NewInComingContext(req.Metadata)
		closeConn := false
		if !req.IsHeartBeat() {
			err = s.auth(ctx, req)
			closeConn = err != nil
		}

		if err != nil {
			res := req.Clone()
			res.SetMessageType(protocol.Response)
			s.handleError(res, err)
			s.sendResponse(conn, err, req, res, nil)

			// auth failed, closed the connection
			if closeConn {
				zap.L().Info("auth failed for conn:", zap.String("addr", conn.RemoteAddr().String()), zap.Error(err))
				return
			}
			continue
		}

		go s.processOneRequest(ctx, req, conn)
	}
}

func (s *Server) readRequest(r io.Reader) (*protocol.Message, error) {
	req := protocol.NewMessage()
	err := req.Decode(r, s.opt.maxReceiveMessageSize)
	if err != nil {
		return nil, err
	}
	if err == io.EOF {
		return req, err
	}
	return req, err
}

func (s *Server) processOneRequest(ctx context.Context, req *protocol.Message, conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1024)
			buf = buf[:runtime.Stack(buf, true)]
			zap.L().Error(fmt.Sprintf("[handler internal error]: servicepath: %s, servicemethod: %s, err: %v，stacks: %s", req.ServiceName, req.ServiceMethod, r, string(buf)))
		}
	}()

	atomic.AddInt32(&s.handleMsgCount, 1)
	defer atomic.AddInt32(&s.handleMsgCount, -1)

	// if heartbeat return directly
	if req.IsHeartBeat() {
		res := req.Clone()
		res.SetMessageType(protocol.Response)
		data := req.Encode()

		if s.opt.writeTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
		}
		conn.Write(*data)
	}

	var err error
	var reply any
	// get service
	svc, ok := s.serviceMap.Load(req.ServiceName)
	srv, _ := svc.(*service)
	if !ok {
		err = errors.New("rpcx: can't find service " + req.ServiceName)
	}
	d := codec.GetBufferSliceFromRequest(req)
	res := req.Clone()
	if md, ok := srv.methods[req.ServiceMethod]; ok {
		res.SetMessageType(protocol.Response)

		df := func(v any) error {
			if err := s.getCodec().Unmarshal(d, v); err != nil {
				return fmt.Errorf("zrpc: error unmarshalling request: %v", err)
			}

			// TODO
			// StatsHandler

			return nil
		}
		ctx = context.WithValue(ctx, responseKey{}, res)

		reply, err = md.Handler(srv.serviceImpl, ctx, df)
		if err != nil {
			s.handleError(res, err)
		}
		if err != nil {
			zap.L().Error("rpcx: failed to handle request: ", zap.Error(err))
		}
	}

	s.sendResponse(conn, err, req, res, reply)
}

func (s *Server) sendResponse(conn net.Conn, err error, req, res *protocol.Message, reply any) {
	d, appErr := s.getCodec().Marshal(reply)
	if appErr != nil {
		err = appErr
	}
	defer codec.PutBufferSlice(&d)
	res.Payload = d.ToBytes()
	if len(res.Payload) > 1024 && res.GetCompressType() != protocol.None {
		res.SetCompressType(req.GetCompressType())
	}

	data := res.Encode()

	go func() {
		if s.opt.writeTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
		}
		_, writeErr := conn.Write(*data)
		if writeErr != nil {
			zap.L().Error("zrpc: failed to send response", zap.Error(writeErr))
		}
	}()
}

func (s *Server) handleError(res *protocol.Message, err error) {
	res.SetMessageStatusType(protocol.Error)
	var key, val string

	key = protocol.ServiceError
	if s.opt.ServerErrorFunc != nil {
		val = s.opt.ServerErrorFunc(res, err)
	} else {
		val = err.Error()
	}

	if res.Metadata.Len() == 0 {
		res.Metadata = metadata.New(map[string]string{key: val})
	} else {
		res.Metadata = metadata.Join(res.Metadata, metadata.Pairs(key, val))
	}
}

func (s *Server) auth(ctx context.Context, req *protocol.Message) error {
	if s.opt.AuthFunc != nil {
		token := req.Metadata[share.AuthKey]
		return s.opt.AuthFunc(ctx, req, token[0])
	}

	return nil
}

func (s *Server) Stop() {
	s.stop(false)
}

func (s *Server) GracefulStop() {
	s.stop(true)
}

func (s *Server) stop(graceful bool) {
	s.startShutdown()
	s.mu.Lock()
	s.lis.Close()
	s.mu.Unlock()

	s.serveWG.Wait()

	if graceful {
		s.mu.Lock()
		defer s.mu.Unlock()

		for len(s.conns) > 0 {
			s.cv.Wait()
		}
	}

	s.conns = nil
}

func (s *Server) isShutDown() bool {
	return atomic.LoadInt32(&s.inShutdown) == 1
}

func (s *Server) startShutdown() {
	if atomic.CompareAndSwapInt32(&s.inShutdown, 0, 1) {
		close(s.done)
	}
}

func (s *Server) removeConn(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.cv.Broadcast()
	s.mu.Unlock()

	conn.Close()
}

func (s *Server) getCodec() codec.Codec {
	return codec.DefaultCodec
}

type responseKey struct{}

func SetMeta(ctx context.Context, md metadata.MD) error {
	if md.Len() == 0 {
		return nil
	}

	res, ok := ctx.Value(responseKey{}).(*protocol.Message)
	if !ok {
		return fmt.Errorf("zrpc failed to fetch response message from context: %v", ctx)
	}

	res.Metadata = metadata.Join(res.Metadata, md)
	return nil
}

func isRecoverableError(err error) bool {
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EINTR) {
		return true
	}
	return false
}
