package zrpc

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"

	"github.com/crazyfrankie/zrpc/codec"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int       // MagicNumber marks this is s a zrpc request
	CodecType   codec.Typ // client may choose different Codec to encode body
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// Server represents an RPC Server.
type Server struct {
	mu         sync.Mutex
	serviceMap sync.Map
}

// NewServer returns a new rpc server
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

func (s *Server) Register(receiver any) error {
	svc := newService(receiver)
	if _, dup := s.serviceMap.LoadOrStore(svc.name, svc); dup {
		return errors.New("rpc: service already defined: " + svc.name)
	}
	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(receiver interface{}) error { return DefaultServer.Register(receiver) }

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (s *Server) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err.Error())
			return
		}

		go s.ServeConn(conn)
	}
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

func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
func (s *Server) ServeConn(conn net.Conn) {
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error:", err.Error())
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number:%x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server invalid codec type:%s", opt.CodecType)
		return
	}

	s.ServeCodec(f(conn))
}

// request stores all information of a call
type request struct {
	h        *codec.Header // header of request
	argVal   reflect.Value // argVal of request
	replyVal reflect.Value // replyVal of request
	mType    *methodType
	svc      *service
}

// invalidRequest is a placeholder for response argv when error occurs
var invalidRequest = struct{}{}

func (s *Server) ServeCodec(cc codec.Codec) {
	var wg sync.WaitGroup

	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Err = err.Error()
			s.sendResponse(cc, req.h, invalidRequest)
			continue
		}
		wg.Add(1)

		// handleRequest
		go func() {
			req.replyVal, err = req.svc.call(req.mType, req.argVal)
			if err != nil {
				req.h.Err = err.Error()
				s.sendResponse(cc, req.h, invalidRequest)
				return
			}
			s.sendResponse(cc, req.h, req.replyVal.Interface())
			wg.Done()
		}()
	}

	wg.Wait()
	_ = cc.Close()
}

func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}

	req := &request{h: h}
	req.svc, req.mType, err = s.findService(h.ServiceName)
	if err != nil {
		return req, err
	}
	req.argVal = req.mType.newArgv()
	req.replyVal = req.mType.newReplyVal()

	// make sure that argv is a pointer, ReadBody need a pointer as parameter
	argv := req.argVal.Interface()
	if req.argVal.Type().Kind() != reflect.Ptr {
		argv = req.argVal.Addr().Interface()
	}
	if err = cc.ReadBody(argv); err != nil {
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}

	return &h, nil
}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}
