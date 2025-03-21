package zrpc

import (
	"bufio"
	"crypto/tls"
	"net"
	"time"

	"go.uber.org/zap"
)

const (
	// ReaderBufferSize is used for bufio reader.
	ReaderBufferSize = 16 * 1024
)

// clientConn represents actual connection with target machine,
// and it based on tcp.
type clientConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	addr     string
	lastUsed time.Time
	inUse    bool
}

func newClientConn(c *Client, target string) (*clientConn, error) {
	var conn net.Conn
	var err error
	var tlsConn *tls.Conn

	if c.opt.tls != nil {
		dialer := &net.Dialer{
			Timeout: c.opt.connectTimeout,
		}
		tlsConn, err = tls.DialWithDialer(dialer, "tcp", target, c.opt.tls)
		conn = net.Conn(tlsConn)
	} else {
		conn, err = net.DialTimeout("tcp", target, c.opt.connectTimeout)
	}

	if err != nil {
		zap.L().Warn("failed to dial server: ", zap.Error(err))
		return nil, err
	}
	if conn != nil {
		if tc, ok := conn.(*net.TCPConn); ok && c.opt.tcpKeepAlivePeriod > 0 {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(c.opt.tcpKeepAlivePeriod)
		}

		if c.opt.idleTimeout != 0 {
			_ = conn.SetDeadline(time.Now().Add(c.opt.idleTimeout))
		}
	}

	r := bufio.NewReaderSize(conn, ReaderBufferSize)

	return &clientConn{
		conn:     conn,
		addr:     target,
		lastUsed: time.Now(),
		reader:   r,
	}, nil
}

func (cc *clientConn) Close() error {
	return cc.conn.Close()
}
