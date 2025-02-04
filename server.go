package zrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
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
	mu sync.Mutex
}

// NewServer returns a new rpc server
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

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
			// TODO, should call registered rpc methods to get the right replyVal
			// day 1, just print argv and send a hello message
			log.Println(req.h, req.argVal.Elem())
			req.replyVal = reflect.ValueOf(fmt.Sprintf("zrpc resp %d", req.h.Seq))
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
	// TODO: now we don't know the type of request argv
	req.argVal = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argVal.Interface()); err != nil {
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
