package transport

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/crazyfrankie/zrpc/peer"
)

type TcpServer struct {
	opt       *transportOpt
	conn      net.Conn
	mu        sync.Mutex
	closeChan chan struct{}
}

func NewServerTransport(conn net.Conn, opts ...Option) *TcpServer {
	opt := &transportOpt{}
	for _, o := range opts {
		o(opt)
	}

	return &TcpServer{
		opt:       opt,
		conn:      conn,
		closeChan: make(chan struct{}),
	}
}

func (st *TcpServer) HandleStreams(ctx context.Context, f func(*ServerStream)) {
	//TODO implement me
	panic("implement me")
}

func (st *TcpServer) Close(err error) {
	//TODO implement me
	panic("implement me")
}

func (st *TcpServer) Peer() *peer.Peer {
	//TODO implement me
	panic("implement me")
}

func (st *TcpServer) Drain(debugData string) {
	//TODO implement me
	panic("implement me")
}

func (st *TcpServer) SetTimeouts() {
	if st.opt.connectionTimeout > 0 {
		st.conn.SetDeadline(time.Now().Add(st.opt.connectionTimeout))
	}
	if st.opt.readTimeout > 0 {
		st.conn.SetReadDeadline(time.Now().Add(st.opt.readTimeout))
	}
	if st.opt.writeTimeout > 0 {
		st.conn.SetWriteDeadline(time.Now().Add(st.opt.writeTimeout))
	}
}

func (st *TcpServer) WriteFrame(data []byte) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	// 计算消息长度
	length := uint32(len(data))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)

	// 先写入长度字段，再写入数据
	if _, err := st.conn.Write(header); err != nil {
		return err
	}
	if _, err := st.conn.Write(data); err != nil {
		return err
	}
	return nil
}

func (st *TcpServer) ReadFrame() ([]byte, error) {
	header := make([]byte, 4)

	// 读取固定长度的消息头
	if _, err := io.ReadFull(st.conn, header); err != nil {
		return nil, err
	}

	// 解析数据长度
	length := binary.BigEndian.Uint32(header)
	payload := make([]byte, length)

	// 读取实际数据
	if _, err := io.ReadFull(st.conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func EncodeMessage(msg proto.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func DecodeMessage(data []byte, msg proto.Message) error {
	return proto.Unmarshal(data, msg)
}
