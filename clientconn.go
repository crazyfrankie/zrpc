package zrpc

import (
	"bufio"
	"crypto/tls"
	"net"
	"sync"
	"time"
)

const (
	// ReaderBufferSize is used for bufio reader.
	ReaderBufferSize = 16 * 1024
)

type connPool struct {
	mu        sync.Mutex
	conns     []*clientConn
	target    string
	dialer    *Client
	size      int
	created   int
	available chan *clientConn
}

func newConnPool(client *Client, target string, size int) *connPool {
	return &connPool{
		conns:     make([]*clientConn, 0, size),
		target:    target,
		dialer:    client,
		size:      size,
		available: make(chan *clientConn, size),
	}
}

func (p *connPool) get() (*clientConn, error) {
	for i := 0; i < 3; i++ {
		select {
		case conn := <-p.available:
			if conn != nil && !conn.closed {
				return conn, nil
			}
		default:
		}

		p.mu.Lock()
		if p.created < p.size {
			conn, err := newClientConn(p.dialer, p.target)
			if err != nil {
				p.mu.Unlock()
				continue
			}
			p.conns = append(p.conns, conn)
			p.created++
			p.mu.Unlock()
			return conn, nil
		}
		p.mu.Unlock()

		select {
		case conn := <-p.available:
			if conn != nil && !conn.closed {
				return conn, nil
			}
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
	return nil, ErrNoAvailableConn
}
func (p *connPool) put(cc *clientConn) {
	if cc == nil || cc.closed {
		return
	}

	select {
	case p.available <- cc:
	default:
		cc.Close()
		p.mu.Lock()
		p.created--
		p.mu.Unlock()
	}
}

type clientConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	addr     string
	lastUsed time.Time
	closed   bool
	mu       sync.Mutex
}

func newClientConn(c *Client, target string) (*clientConn, error) {
	var conn net.Conn
	var err error

	if c.opt.tls != nil {
		dialer := &net.Dialer{
			Timeout: c.opt.connectTimeout,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", target, c.opt.tls)
	} else {
		conn, err = net.DialTimeout("tcp", target, c.opt.connectTimeout)
	}

	if err != nil {
		return nil, err
	}

	if tc, ok := conn.(*net.TCPConn); ok && c.opt.tcpKeepAlivePeriod > 0 {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(c.opt.tcpKeepAlivePeriod)
	}

	return &clientConn{
		conn:     conn,
		addr:     target,
		reader:   bufio.NewReaderSize(conn, ReaderBufferSize),
		lastUsed: time.Now(),
	}, nil
}

func (cc *clientConn) Close() error {
	cc.mu.Lock()
	if cc.closed {
		cc.mu.Unlock()
		return nil
	}
	cc.closed = true
	cc.mu.Unlock()
	return cc.conn.Close()
}
