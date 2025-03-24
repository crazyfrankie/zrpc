package zrpc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ReaderBufferSize is used for bufio reader.
	ReaderBufferSize = 16 * 1024

	connStatusIdle = iota
	connStatusInUse
	connStatusClosed
)

type connPool struct {
	mu      sync.RWMutex
	conns   []*clientConn
	target  string
	dialer  *Client
	size    int
	created int32

	hotQueue  chan *clientConn // For fast connection acquisition (no locks)
	coldQueue []*clientConn    // Alternate connection pooling (lock-protected)

	closed bool

	prewarmed atomic.Bool
}

func newConnPool(client *Client, target string, size int) *connPool {
	// Set the queue size to half the total size of the connection pool
	hotQueueSize := size / 2
	if hotQueueSize < 1 {
		hotQueueSize = 1
	}

	pool := &connPool{
		conns:     make([]*clientConn, 0, size),
		target:    target,
		dialer:    client,
		size:      size,
		hotQueue:  make(chan *clientConn, hotQueueSize),
		coldQueue: make([]*clientConn, 0, size-hotQueueSize),
	}

	// Asynchronous warm-up connection pooling
	go pool.prewarmConnections()

	return pool
}

// prewarmConnections connection Pool Warmup Functions
func (p *connPool) prewarmConnections() {
	// prewarm 1/4 connection
	prewarmCount := p.size / 4
	if prewarmCount < 1 {
		prewarmCount = 1
	}

	for i := 0; i < prewarmCount; i++ {
		if p.closed {
			return
		}

		conn, err := newClientConn(p.dialer, p.target)
		if err != nil {
			continue
		}

		p.mu.Lock()
		p.conns = append(p.conns, conn)
		atomic.AddInt32(&p.created, 1)
		p.coldQueue = append(p.coldQueue, conn)
		p.mu.Unlock()
	}

	// Mark warm-up complete
	p.prewarmed.Store(true)
}

// get returns an active conn with optimization for high concurrency
func (p *connPool) get() (*clientConn, error) {
	if p.closed {
		return nil, ErrClientConnClosing
	}

	// first try to get a connection from the hot queue (lock-free path)
	select {
	case conn := <-p.hotQueue:
		if conn != nil && !conn.isClosed() && conn.isHealthy() {
			conn.markInUse()
			return conn, nil
		}

		// unhealthy connection, close and recycle
		if conn != nil {
			p.releaseConn(conn)
		}
	default:
		// hot list is empty, keep trying to get it another way
	}

	// check if you can create a new connection directly
	currentCreated := atomic.LoadInt32(&p.created)
	if currentCreated < int32(p.size) {
		if conn, err := p.createNewConn(); err == nil {
			return conn, nil
		}
	}

	// Trying to get a connection from the cold queue
	p.mu.Lock()
	if len(p.coldQueue) > 0 {
		conn := p.coldQueue[len(p.coldQueue)-1]
		p.coldQueue = p.coldQueue[:len(p.coldQueue)-1]
		p.mu.Unlock()

		if conn != nil && !conn.isClosed() && conn.isHealthy() {
			conn.markInUse()
			return conn, nil
		}

		// unhealthy connection, close and recycle
		p.releaseConn(conn)
	} else {
		p.mu.Unlock()
	}

	// try the hot queue again, possibly with a released connection.
	select {
	case conn := <-p.hotQueue:
		if conn != nil && !conn.isClosed() && conn.isHealthy() {
			conn.markInUse()
			return conn, nil
		}

		if conn != nil {
			p.releaseConn(conn)
		}
	default:
	}

	p.mu.Lock()
	currentCreated = atomic.LoadInt32(&p.created)
	if currentCreated < int32(p.size) {
		p.mu.Unlock()
		if conn, err := p.createNewConn(); err == nil {
			return conn, nil
		}
	} else {
		p.mu.Unlock()
	}

	var conn *clientConn
	baseWait := 5 * time.Millisecond
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		waitTime := baseWait * time.Duration(1<<uint(i))

		select {
		case conn = <-p.hotQueue:
			if conn != nil && !conn.isClosed() && conn.isHealthy() {
				conn.markInUse()
				return conn, nil
			}
			if conn != nil {
				p.releaseConn(conn)
			}
		case <-time.After(waitTime):
			continue
		}
	}

	return nil, ErrNoAvailableConn
}

// createNewConn Creates a new connection
func (p *connPool) createNewConn() (*clientConn, error) {
	p.mu.Lock()
	// Double-checking to avoid over-creation
	if atomic.LoadInt32(&p.created) >= int32(p.size) {
		p.mu.Unlock()
		return nil, ErrNoAvailableConn
	}

	conn, err := newClientConn(p.dialer, p.target)
	if err != nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("failed to create new connection: %w", err)
	}

	p.conns = append(p.conns, conn)
	atomic.AddInt32(&p.created, 1)
	p.mu.Unlock()

	conn.markInUse()
	return conn, nil
}

func (p *connPool) releaseConn(conn *clientConn) {
	if conn == nil {
		return
	}

	conn.Close()
	atomic.AddInt32(&p.created, -1)
}

// Optimize the put method to prioritize hot queue placement
func (p *connPool) put(cc *clientConn) {
	if cc == nil || cc.isClosed() {
		return
	}

	cc.markIdle()
	cc.lastUsed = time.Now()

	// Prioritize attempts to place in hot queue (lock-free path)
	select {
	case p.hotQueue <- cc:
		return
	default:
		// Hot queue is full
	}

	// Hot queue is full, try to put in cold queue
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		cc.Close()
		atomic.AddInt32(&p.created, -1)
		return
	}

	p.coldQueue = append(p.coldQueue, cc)
}

// Close Closing the Connection Pool
func (p *connPool) Close() {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return
	}

	p.closed = true

	conns := p.conns
	p.conns = nil

	coldConns := p.coldQueue
	p.coldQueue = nil

	p.mu.Unlock()

	for _, conn := range conns {
		if conn != nil {
			conn.Close()
		}
	}

	for _, conn := range coldConns {
		if conn != nil {
			conn.Close()
		}
	}

	close(p.hotQueue)
	for conn := range p.hotQueue {
		if conn != nil && !conn.isClosed() {
			conn.Close()
		}
	}

	atomic.StoreInt32(&p.created, 0)
}

type clientConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	addr     string
	lastUsed time.Time
	status   int32 // connection status
	mu       sync.Mutex

	useCount    int32         // Use Counting
	errorCount  int32         // error count
	healthScore int32         // Health score (0-100)
	createTime  time.Time     // Creation time
	pingTime    time.Duration // RTT of last ping
}

func newClientConn(c *Client, target string) (*clientConn, error) {
	// 创建TCP连接
	var conn net.Conn
	var err error

	if c.opt.connectTimeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), c.opt.connectTimeout)
		defer cancel()

		d := &net.Dialer{}
		conn, err = d.DialContext(ctx, "tcp", target)
	} else {
		conn, err = net.Dial("tcp", target)
	}

	if err != nil {
		return nil, err
	}

	if c.opt.tcpKeepAlivePeriod > 0 {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(c.opt.tcpKeepAlivePeriod)
		}
	}

	now := time.Now()
	cc := &clientConn{
		conn:        conn,
		reader:      bufio.NewReaderSize(conn, ReaderBufferSize),
		addr:        target,
		lastUsed:    now,
		status:      connStatusIdle,
		healthScore: 100,
		createTime:  now,
	}

	return cc, nil
}

func (cc *clientConn) Close() error {
	if cc == nil || cc.conn == nil {
		return nil
	}

	cc.mu.Lock()
	if atomic.LoadInt32(&cc.status) == connStatusClosed {
		cc.mu.Unlock()
		return nil
	}

	atomic.StoreInt32(&cc.status, connStatusClosed)
	cc.mu.Unlock()

	return cc.conn.Close()
}

func (cc *clientConn) isClosed() bool {
	return atomic.LoadInt32(&cc.status) == connStatusClosed
}

func (cc *clientConn) markInUse() {
	atomic.StoreInt32(&cc.status, connStatusInUse)
	atomic.AddInt32(&cc.useCount, 1)
}

func (cc *clientConn) markIdle() {
	atomic.StoreInt32(&cc.status, connStatusIdle)
	cc.lastUsed = time.Now()
}

func (cc *clientConn) isHealthy() bool {
	if cc.isClosed() {
		return false
	}

	maxConnAge := 1 * time.Hour
	if time.Since(cc.createTime) > maxConnAge {
		return false
	}

	if atomic.LoadInt32(&cc.healthScore) < 50 {
		return false
	}

	if time.Since(cc.lastUsed) > 5*time.Minute {
		return cc.pingCheck()
	}

	return true
}

// pingCheck Health check via ping
func (cc *clientConn) pingCheck() bool {
	err := cc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		return false
	}

	// Try to read in non-blocking mode
	one := make([]byte, 1)
	conn := cc.conn

	// Try to read, expect a timeout
	_, err = conn.Read(one)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// This is the expected timeout error, the connection is good
			// Restore the original timeout
			cc.conn.SetReadDeadline(time.Time{}) // Clear the timeout setting
			return true
		}

		// other errors indicate that the connection has been disconnected
		cc.decreaseHealth(50)
		return false
	}

	cc.conn.SetReadDeadline(time.Time{})

	return true
}

// increaseHealth Increase Connected Health Score
func (cc *clientConn) increaseHealth(delta int32) {
	for {
		current := atomic.LoadInt32(&cc.healthScore)
		next := current + delta
		if next > 100 {
			next = 100
		}
		if atomic.CompareAndSwapInt32(&cc.healthScore, current, next) {
			break
		}
	}
}

// decreaseHealth Decrease Connection Health Score
func (cc *clientConn) decreaseHealth(delta int32) {
	for {
		current := atomic.LoadInt32(&cc.healthScore)
		next := current - delta
		if next < 0 {
			next = 0
		}
		if atomic.CompareAndSwapInt32(&cc.healthScore, current, next) {
			break
		}
	}
}
