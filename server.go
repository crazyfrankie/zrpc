package zrpc

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/crazyfrankie/zrpc/codec"
	"github.com/crazyfrankie/zrpc/mem"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
	"github.com/crazyfrankie/zrpc/share"
)

const (
	ReadBufferSize = 1024
)

type task struct {
	ctx    context.Context
	req    *protocol.Message
	conn   net.Conn
	server *Server
}

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

	dynamicPool *workPool
	addr        string
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

	if opt.enableWorkerPool {
		s.initWorkerPool()
	}

	return s
}

// worker in zRPC is the upper abstraction of the goroutine,
// a worker represents a reused concurrent process, the main reason for this is that when high concurrency,
// each request opens a goroutine resulting in a large amount of goroutine creation and destruction,
// affecting performance.
type worker struct {
	tasks  chan task
	quit   chan struct{}
	server *Server
	id     int
}

// newWorker returns a new worker
func newWorker(id int, server *Server) *worker {
	return &worker{
		tasks:  make(chan task),
		quit:   make(chan struct{}),
		server: server,
		id:     id,
	}
}

// start starts a worker to begin working
func (w *worker) start() {
	go func() {
		for {
			select {
			case t := <-w.tasks:
				w.server.doProcessOneRequest(t.ctx, t.req, t.conn)
				if pool := w.server.dynamicPool; pool != nil {
					atomic.AddInt32(&pool.workerLoads[w.id], -1)
				}
			case <-w.quit:
				return
			}
		}
	}()
}

// stop stops a worker
func (w *worker) stop() {
	close(w.quit)
}

// A workPool is an abstraction of a set of workers that manages the creation, scheduling, and destruction of workers.
type workPool struct {
	minWorkers     int
	maxWorkers     int
	currentWorkers int32
	taskQueue      chan task
	workers        []*worker
	metrics        *PoolMetrics
	adjustInterval time.Duration
	mu             sync.RWMutex
	server         *Server

	workerLoads     []int32
	lastAdjustTime  time.Time
	adjustThreshold float64
}

// PoolMetrics represent the load metrics of the workers in a pool
// and are used for dynamic scaling.These include task load counts,
// average latency, request success rate, CPU and Memory utilization.
type PoolMetrics struct {
	queueUsage     float64
	idleWorkers    float64
	cpuUsage       float64
	memoryUsage    float64
	avgLatency     float64
	successRate    float64
	lastAdjustTime time.Time
}

func newWorkPool(server *Server, minWorkers, maxWorkers int, queueSize int) *workPool {
	pool := &workPool{
		minWorkers:      minWorkers,
		maxWorkers:      maxWorkers,
		currentWorkers:  int32(minWorkers),
		taskQueue:       make(chan task, queueSize),
		workers:         make([]*worker, 0, maxWorkers),
		metrics:         &PoolMetrics{lastAdjustTime: time.Now()},
		adjustInterval:  time.Second * 5,
		server:          server,
		workerLoads:     make([]int32, maxWorkers),
		adjustThreshold: 0.8, // Trigger adjustment at 80% load, also allows user decision making
	}

	// Initially start only the smallest worker thread to avoid wasting resources.
	// Can be expanded through later asynchronous detection
	for i := 0; i < minWorkers; i++ {
		w := newWorker(i, server)
		pool.workers = append(pool.workers, w)
		w.start()
	}

	// Start the dynamic adjustment co-process
	go pool.adjustWorkers()

	// Start the Task Distribution Concatenation
	go pool.dispatch()

	return pool
}

// dispatch is responsible for distributing tasks
// and dynamically determining the load on the worker to balance after load balancing
// (since the Client has already done something similar by picking the Server
// to send the request through a load balancing policy).
func (p *workPool) dispatch() {
	for t := range p.taskQueue {
		workerIndex := p.selectWorker()
		if workerIndex >= 0 {
			p.mu.RLock()
			if workerIndex < len(p.workers) {
				w := p.workers[workerIndex]
				select {
				case w.tasks <- t:
					atomic.AddInt32(&p.workerLoads[workerIndex], 1)
					continue
				default:
					// The worker thread is busy, move on to the next one.
				}
			}
			p.mu.RUnlock()
		}

		// If all workers are busy, use the fallback policy
		p.handleOverload(t)
	}
}

// selectWorker dynamically selects the optimal executing worker
// from the load of the workers recorded in real time.
func (p *workPool) selectWorker() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.workers) == 0 {
		return -1
	}

	// Find the least loaded worker thread
	minLoad := int32(math.MaxInt32)
	selectedIndex := -1

	for i, load := range p.workerLoads {
		if i >= len(p.workers) {
			break
		}
		if load < minLoad {
			minLoad = load
			selectedIndex = i
		}
	}

	return selectedIndex
}

// handleOverload handles the state where all workers are busy,
// determines whether to expand the queue based on the current load,
// and if the expansion is successful, uses the expanded worker to handle it,
// otherwise it directly tries to start a new goroutine to execute the task.
func (p *workPool) handleOverload(t task) {
	if p.metrics.queueUsage > p.adjustThreshold {
		p.quickScaleUp()
	}

	p.mu.RLock()
	for i, w := range p.workers {
		select {
		case w.tasks <- t:
			atomic.AddInt32(&p.workerLoads[i], 1)
			p.mu.RUnlock()
			return
		default:
			continue
		}
	}
	p.mu.RUnlock()

	// If still unassigned, deal with it directly
	go p.server.doProcessOneRequest(t.ctx, t.req, t.conn)
}

// quickScaleUp is an emergency braking strategy
// that protects the system's security mechanisms
// by turning on a large number of workers at once
// while staying within the maximum tolerable number of workers when unexpected high concurrency traffic hits.
func (p *workPool) quickScaleUp() {
	currentWorkers := int(atomic.LoadInt32(&p.currentWorkers))
	if currentWorkers >= p.maxWorkers {
		return
	}

	// Rapidly increase work threads by 20%
	targetWorkers := int(float64(currentWorkers) * 1.2)
	if targetWorkers > p.maxWorkers {
		targetWorkers = p.maxWorkers
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i := currentWorkers; i < targetWorkers; i++ {
		w := newWorker(i, p.server)
		p.workers = append(p.workers, w)
		w.start()
		atomic.AddInt32(&p.currentWorkers, 1)
	}
}

// adjustWorkers asynchronous policy to dynamically monitor and update the status of each worker,
// while fine-tuning the number of workers based on the current load.
func (p *workPool) adjustWorkers() {
	ticker := time.NewTicker(p.adjustInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.updateMetrics()
		p.adjustWorkerCount()
	}
}

// updateMetrics Timed task to update worker load metrics for daily fine-tuning.
func (p *workPool) updateMetrics() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Update queue utilization
	queueLen := len(p.taskQueue)
	queueCap := cap(p.taskQueue)
	p.metrics.queueUsage = float64(queueLen) / float64(queueCap)

	// Update a worker thread load
	totalLoad := int32(0)
	for i := range p.workerLoads {
		if i < len(p.workers) {
			load := atomic.LoadInt32(&p.workerLoads[i])
			totalLoad += load
		}
	}

	if len(p.workers) > 0 {
		p.metrics.idleWorkers = 1.0 - float64(totalLoad)/float64(len(p.workers))
	}

	// Update system resource utilization
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.metrics.cpuUsage = float64(m.Sys) / float64(runtime.NumCPU()*1024*1024)
	p.metrics.memoryUsage = float64(m.Alloc) / float64(m.Sys)
}

// adjustWorkerCount is a daily adjustment strategy, unlike quickScaleUp,
// which only fine-tunes the number of workers based on system runtime timer detection,
// and is not able to cope with highly concurrent traffic, but is a simple strategy to save resources.
func (p *workPool) adjustWorkerCount() {
	currentWorkers := int(atomic.LoadInt32(&p.currentWorkers))
	targetWorkers := currentWorkers

	// Adjust the number of worker threads to the load
	if p.metrics.queueUsage > p.adjustThreshold && p.metrics.idleWorkers < 0.2 {
		// High load and few idle threads, increase worker threads
		targetWorkers = int(float64(currentWorkers) * 1.2)
	} else if p.metrics.queueUsage < 0.2 && p.metrics.idleWorkers > 0.8 {
		// Low load and many idle threads, fewer worker threads
		targetWorkers = int(float64(currentWorkers) * 0.8)
	}

	// Ensure that the minimum and maximum ranges
	if targetWorkers < p.minWorkers {
		targetWorkers = p.minWorkers
	} else if targetWorkers > p.maxWorkers {
		targetWorkers = p.maxWorkers
	}

	// If adjustments are needed
	if targetWorkers != currentWorkers {
		p.mu.Lock()
		defer p.mu.Unlock()

		if targetWorkers > currentWorkers {
			// Add worker threads
			for i := currentWorkers; i < targetWorkers; i++ {
				w := newWorker(i, p.server)
				p.workers = append(p.workers, w)
				w.start()
				atomic.AddInt32(&p.currentWorkers, 1)
			}
		} else {
			// Reduce work threads
			for i := currentWorkers - 1; i >= targetWorkers; i-- {
				if i < len(p.workers) {
					p.workers[i].stop()
					p.workers = p.workers[:i]
					atomic.AddInt32(&p.currentWorkers, -1)
				}
			}
		}
	}
}

// stop shuts down the work pool
func (p *workPool) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, w := range p.workers {
		w.stop()
	}
	close(p.taskQueue)
}

// initWorkerPool Initialize the work pool
func (s *Server) initWorkerPool() {
	if s.opt.enableWorkerPool {
		s.dynamicPool = newWorkPool(s,
			s.opt.minWorkerPoolSize,
			s.opt.maxWorkerPoolSize,
			s.opt.taskQueueSize)
	}
}

// Serve starts and listens to RPC requests.
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
			zap.L().Error(fmt.Sprintf("zrpc: TLS handshake error from %s: %v", conn.RemoteAddr(), err))
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
				zap.L().Warn("zrpc: failed to read request: ", zap.String("err", err.Error()))
			}

			return
		}

		// inject metadata to context
		ctx = metadata.NewInComingContext(ctx, req.Metadata)
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

		if s.opt.enableWorkerPool {
			select {
			case s.dynamicPool.taskQueue <- task{ctx: ctx, req: req, conn: conn, server: s}:
			default:
				// queue full, process directly
				go s.processOneRequest(ctx, req, conn)
			}
		} else {
			go s.processOneRequest(ctx, req, conn)
		}
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

// doProcessOneRequest is the encapsulated workpool's processing method.
func (s *Server) doProcessOneRequest(ctx context.Context, req *protocol.Message, conn net.Conn) {
	s.processOneRequest(ctx, req, conn)
}

// processOneRequest raw processing request method
func (s *Server) processOneRequest(ctx context.Context, req *protocol.Message, conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1024)
			buf = buf[:runtime.Stack(buf, true)]
			zap.L().Error(fmt.Sprintf("[handler memory error]: servicepath: %s, servicemethod: %s, err: %v，stacks: %s", req.ServiceName, req.ServiceMethod, r, string(buf)))
		}
	}()

	atomic.AddInt32(&s.handleMsgCount, 1)
	defer atomic.AddInt32(&s.handleMsgCount, -1)

	// if heartbeat return directly
	if req.IsHeartBeat() {
		res := req.Clone()
		res.SetMessageType(protocol.Response)
		msgBuffer := res.Encode()

		if s.opt.writeTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
		}
		conn.Write(msgBuffer.ReadOnlyData())
		msgBuffer.Free()
	}

	var err error
	var reply any
	// get service
	svc, ok := s.serviceMap.Load(req.ServiceName)
	srv, _ := svc.(*service)
	if !ok {
		err = errors.New("zrpc: can't find service " + req.ServiceName)
	}

	res := req.Clone()
	if md, ok := srv.methods[req.ServiceMethod]; ok {
		res.SetMessageType(protocol.Response)

		df := func(v any) error {
			if err := s.getCodec().Unmarshal(mem.BufferSlice{mem.SliceBuffer(req.Payload)}, v); err != nil {
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
			zap.L().Error("zrpc: failed to handle request: ", zap.Error(err))
		}
	}

	s.sendResponse(conn, err, req, res, reply)
}

func (s *Server) sendResponse(conn net.Conn, err error, req, res *protocol.Message, reply any) {
	var d mem.BufferSlice
	var appErr error

	if reply != nil {
		d, appErr = s.getCodec().Marshal(reply)
		if appErr != nil {
			err = appErr
		}
	}
	defer d.Free()

	if d.Len() > 0 {
		res.Payload = d.Materialize()
		if len(res.Payload) > 1024 && req.GetCompressType() != protocol.None {
			res.SetCompressType(req.GetCompressType())
		}
	}

	msgBuffer := res.Encode()

	go func() {
		if s.opt.writeTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
		}
		_, writeErr := conn.Write(msgBuffer.ReadOnlyData())
		msgBuffer.Free()
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

	if s.opt.enableWorkerPool {
		s.dynamicPool.stop()
	}

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

// MethodInfo contains the information of an RPC including its method name and type.
type MethodInfo struct {
	Name string
}

type ServiceInfo struct {
	Methods []MethodInfo
	// Metadata is the metadata specified in ServiceDesc when registering service.
	Metadata any
}

func (s *Server) GetServiceInfo() map[string]ServiceInfo {
	return nil
}
