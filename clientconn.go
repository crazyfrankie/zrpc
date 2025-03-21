package zrpc

import (
	"bufio"
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ReaderBufferSize is used for bufio reader.
	ReaderBufferSize = 16 * 1024

	// 连接状态常量
	connStatusIdle = iota
	connStatusInUse
	connStatusClosed
)

type connPool struct {
	mu        sync.Mutex
	conns     []*clientConn
	target    string
	dialer    *Client
	size      int
	created   int32
	available chan *clientConn
	closed    bool
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
	// 快速检查是否已关闭
	if p.closed {
		return nil, ErrClientConnClosing
	}

	// 1. 尝试从可用连接池中获取连接
	select {
	case conn := <-p.available:
		if conn != nil && !conn.isClosed() && conn.isHealthy() {
			conn.markInUse()
			return conn, nil
		}
		// 如果连接不健康，关闭并尝试创建新的
		if conn != nil {
			conn.Close()
			atomic.AddInt32(&p.created, -1)
		}
	default:
	}

	// 2. 创建新连接
	p.mu.Lock()
	if atomic.LoadInt32(&p.created) < int32(p.size) {
		conn, err := newClientConn(p.dialer, p.target)
		if err != nil {
			p.mu.Unlock()
			return nil, err
		}
		p.conns = append(p.conns, conn)
		atomic.AddInt32(&p.created, 1)
		p.mu.Unlock()

		conn.markInUse()
		return conn, nil
	}
	p.mu.Unlock()

	// 3. 等待可用连接 (使用指数退避策略)
	var conn *clientConn
	baseWait := 10 * time.Millisecond

	for i := 0; i < 5; i++ {
		waitTime := baseWait * time.Duration(1<<uint(i))

		select {
		case conn = <-p.available:
			if conn != nil && !conn.isClosed() && conn.isHealthy() {
				conn.markInUse()
				return conn, nil
			}
			// 如果连接不健康，关闭并继续等待
			if conn != nil {
				conn.Close()
				atomic.AddInt32(&p.created, -1)
			}
		case <-time.After(waitTime):
			continue
		}
	}

	return nil, ErrNoAvailableConn
}

func (p *connPool) put(cc *clientConn) {
	if cc == nil || cc.isClosed() {
		return
	}

	cc.markIdle()

	select {
	case p.available <- cc:
	default:
		cc.Close()
		atomic.AddInt32(&p.created, -1)
	}
}

func (p *connPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true
	close(p.available)

	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = nil
	atomic.StoreInt32(&p.created, 0)
}

type clientConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	addr     string
	lastUsed time.Time
	status   int32
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
		status:   connStatusIdle,
	}, nil
}

func (cc *clientConn) Close() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.isClosed() {
		return nil
	}

	atomic.StoreInt32(&cc.status, connStatusClosed)
	return cc.conn.Close()
}

func (cc *clientConn) isClosed() bool {
	return atomic.LoadInt32(&cc.status) == connStatusClosed
}

func (cc *clientConn) markInUse() {
	atomic.StoreInt32(&cc.status, connStatusInUse)
	cc.lastUsed = time.Now()
}

func (cc *clientConn) markIdle() {
	atomic.StoreInt32(&cc.status, connStatusIdle)
}

// 检查连接是否健康
func (cc *clientConn) isHealthy() bool {
	if cc.isClosed() {
		return false
	}

	// 如果连接太久没用，可能已经被服务端关闭
	if time.Since(cc.lastUsed) > 5*time.Minute {
		return false
	}

	return true
}
