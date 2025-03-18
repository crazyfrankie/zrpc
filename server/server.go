package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
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

	inShutdown int32
	quit       chan struct{}
}

// NewServer returns a new rpc server
func NewServer(opts ...Option) *Server {
	opt := defaultServerOption
	for _, o := range opts {
		o(opt)
	}

	return &Server{
		opt:   opt,
		conns: make(map[net.Conn]struct{}),
		quit:  make(chan struct{}),
	}
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
				case <-s.quit:
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
			s.serveConn(conn)
			s.serveWG.Done()
		}()
	}
}

// serveConn runs the server on a single connection.
// serveConn blocks, serving the connection until the client hangs up.
func (s *Server) serveConn(conn net.Conn) {
	if s.isShutDown() {
		s.removeConn(conn)
		return
	}
	defer func() {
		// TODO

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

	// r := bufio.NewReaderSize(conn, ReadBufferSize)

	// TODO
	// read requests and handle it
	for {
		if s.isShutDown() {
			return
		}

		now := time.Now()
		if s.opt.readTimeout != 0 {
			conn.SetReadDeadline(now.Add(s.opt.readTimeout))
		}
	}
	//var opt Option
	//if err := json.NewDecoder(conn).Decode(&opt); err != nil {
	//	log.Println("rpc server: options error:", err.Error())
	//	return
	//}
	//if opt.MagicNumber != MagicNumber {
	//	log.Printf("rpc server: invalid magic number:%x", opt.MagicNumber)
	//	return
	//}
	//f := codec.NewCodecFuncMap[opt.CodecType]
	//if f == nil {
	//	log.Printf("rpc server invalid codec type:%s", opt.CodecType)
	//	return
	//}

	//s.ServeCodec(f(conn))
}

func (s *Server) Register(receiver any) error {
	svc := newService(receiver)
	if _, dup := s.serviceMap.LoadOrStore(svc.name, svc); dup {
		return errors.New("rpc: service already defined: " + svc.name)
	}
	return nil
}

func (s *Server) findService(method string) (*service, *methodType, error) {
	var err error
	dot := strings.LastIndex(method, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + method)
		return nil, nil, err
	}

	serviceName, methodName := method[:dot], method[dot+1:]
	svci, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return nil, nil, err
	}
	svc := svci.(*service)
	mType := svc.method[methodName]
	if mType == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}

	return svc, mType, nil
}

func (s *Server) GracefulStop() {
	s.stop()
}

func (s *Server) stop() {
	for len(s.conns) != 0 {
		s.serveWG.Wait()
	}
}

func (s *Server) isShutDown() bool {
	return atomic.LoadInt32(&s.inShutdown) == 1
}

func (s *Server) removeConn(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()

	conn.Close()
}

func (s *Server) readRequest(ctx context.Context, r io.Reader) () {

}

// request stores all information of a call
//type request struct {
//	h        *codec.Header // header of request
//	argVal   reflect.Value // argVal of request
//	replyVal reflect.Value // replyVal of request
//	mType    *methodType
//	svc      *service
//}
//
//// invalidRequest is a placeholder for response argv when error occurs
//var invalidRequest = struct{}{}
//
//func (s *Server) serveSteam(ctx context.Context, st *transport.TcpServer) {
//	//var wg sync.WaitGroup
//
//	//for {
//	//	req, err := s.readRequest(cc)
//	//	if err != nil {
//	//		if req == nil {
//	//			break
//	//		}
//	//		req.h.Err = err.Error()
//	//		s.sendResponse(cc, req.h, invalidRequest)
//	//		continue
//	//	}
//	//	wg.Add(1)
//	//
//	//	// handleRequest
//	//	go func() {
//	//		req.replyVal, err = req.svc.call(req.mType, req.argVal)
//	//		if err != nil {
//	//			req.h.Err = err.Error()
//	//			s.sendResponse(cc, req.h, invalidRequest)
//	//			return
//	//		}
//	//		s.sendResponse(cc, req.h, req.replyVal.Interface())
//	//		wg.Done()
//	//	}()
//	//}
//	//
//	//wg.Wait()
//	//_ = cc.Close()
//}
//
//func (s *Server) readRequest(cc codec.Codec) (*request, error) {
//	h, err := s.readRequestHeader(cc)
//	if err != nil {
//		return nil, err
//	}
//
//	req := &request{h: h}
//	req.svc, req.mType, err = s.findService(h.ServiceName)
//	if err != nil {
//		return req, err
//	}
//	req.argVal = req.mType.newArgv()
//	req.replyVal = req.mType.newReplyVal()
//
//	// make sure that argv is a pointer, ReadBody need a pointer as parameter
//	argv := req.argVal.Interface()
//	if req.argVal.Type().Kind() != reflect.Ptr {
//		argv = req.argVal.Addr().Interface()
//	}
//	if err = cc.ReadBody(argv); err != nil {
//		log.Println("rpc server: read argv err:", err)
//	}
//	return req, nil
//}
//
//func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
//	var h codec.Header
//	if err := cc.ReadHeader(&h); err != nil {
//		if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
//			log.Println("rpc server: read header error:", err)
//		}
//		return nil, err
//	}
//
//	return &h, nil
//}
//
//func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}) {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	if err := cc.Write(h, body); err != nil {
//		log.Println("rpc server: write response error:", err)
//	}
//}
//
func isRecoverableError(err error) bool {
	// 连接被重置、被信号中断，可以重试
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EINTR) {
		return true
	}
	return false
}
