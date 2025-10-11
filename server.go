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
	"github.com/crazyfrankie/zrpc/mem"
	"github.com/crazyfrankie/zrpc/metadata"
	"github.com/crazyfrankie/zrpc/protocol"
	"github.com/crazyfrankie/zrpc/share"
)

const (
	ReadBufferSize = 1024
	// serverWorkerResetThreshold determines how many requests a worker goroutine
	// processes before recycling itself to prevent unbounded stack growth.
	// Using 2^16 (65536).
	serverWorkerResetThreshold = 1 << 16
)

// Server represents an RPC Server.
type Server struct {
	lis        net.Listener
	mu         sync.RWMutex
	opt        *serverOption
	conns      map[net.Conn]struct{}
	serviceMap sync.Map       // service name -> service info
	serveWG    sync.WaitGroup // counts active Serve goroutines for Stop/GracefulStop

	cv             *sync.Cond
	serve          bool
	handleMsgCount int32
	inShutdown     int32
	done           chan struct{}

	// Function-based worker pool
	// Channel of functions to be executed instead of tasks
	workerFuncChannel      chan func()
	workerFuncChannelClose func()
	numWorkers             int32

	// Function to create a new worker - allows for testing via mocking
	workerCreator func()

	poolMetricsMu sync.RWMutex // Protect metrics updates
	metrics       *PoolMetrics // Enhanced dynamic scaling metrics
	workerLoads   []int32      // Record the load of each worker

	// Connection write synchronization
	connMutexesMu sync.Mutex
	connMutexes   map[net.Conn]*sync.Mutex
}

// NewServer returns a new rpc server
func NewServer(opts ...ServerOption) *Server {
	opt := defaultServerOption
	for _, o := range opts {
		o(opt)
	}

	s := &Server{
		opt:         opt,
		conns:       make(map[net.Conn]struct{}),
		done:        make(chan struct{}),
		metrics:     &PoolMetrics{lastAdjustTime: time.Now()},
		workerLoads: make([]int32, opt.maxWorkerPoolSize),
		connMutexes: make(map[net.Conn]*sync.Mutex),
	}

	s.cv = sync.NewCond(&s.mu)

	chainServerMiddlewares(s)

	// Set up the worker creator function
	s.workerCreator = func() {
		nextID := int(atomic.AddInt32(&s.numWorkers, 1) - 1)
		go s.serverWorker(nextID)
	}

	// Override the worker pool initialization
	if s.opt.enableWorkerPool {
		s.initFunctionWorkerPool()
	}

	return s
}

// initFunctionWorkerPool initializes the function-based worker pool
func (s *Server) initFunctionWorkerPool() {
	numWorkers := s.opt.minWorkerPoolSize
	s.numWorkers = int32(numWorkers)
	// Use a buffered channel for the worker functions
	// The buffer size should be large enough to handle bursts of requests
	// but not so large that it consumes too much memory
	s.workerFuncChannel = make(chan func(), s.opt.taskQueueSize)
	s.workerFuncChannelClose = sync.OnceFunc(func() {
		close(s.workerFuncChannel)
	})

	// Set the adjustment threshold above which emergency capacity expansion is triggered
	s.metrics.queueUsage = 0
	s.metrics.idleWorkers = 1.0

	// Start the initial workers based on the minWorkerPoolSize
	s.mu.Lock()
	for i := 0; i < numWorkers; i++ {
		s.workerCreator()
	}
	s.mu.Unlock()

	// Start a monitoring goroutine to dynamically adjust the number of workers
	go s.monitorWorkerPool()
}

// serverWorker implements the gRPC-style worker that processes a certain number
// of functions before recycling itself by spawning a new worker goroutine
func (s *Server) serverWorker(workerID int) {
	// Catch panics in worker goroutines
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("Recovered from panic in serverWorker",
				zap.Int("workerID", workerID),
				zap.Any("panic", r))

			// Spawn a replacement worker to maintain the pool size
			go s.workerCreator()
		}
	}()

	for completed := 0; completed < serverWorkerResetThreshold; completed++ {
		// Use select with a default case to avoid blocking indefinitely
		// if the server is shutting down
		var f func()
		var ok bool

		select {
		case f, ok = <-s.workerFuncChannel:
			if !ok {
				// Channel is closed, server is shutting down
				return
			}
		default:
			// No work available, sleep briefly to avoid CPU spinning
			// and check again
			time.Sleep(time.Millisecond)
			continue
		}

		// Safely update worker load - check array bounds first
		s.mu.RLock()
		withinBounds := workerID >= 0 && workerID < len(s.workerLoads)
		s.mu.RUnlock()

		if withinBounds {
			atomic.AddInt32(&s.workerLoads[workerID], 1)
		}

		// Execute function with panic protection
		func() {
			defer func() {
				if r := recover(); r != nil {
					zap.L().Error("Recovered from panic in worker function",
						zap.Int("workerID", workerID),
						zap.Any("panic", r))
				}
			}()
			f()
		}()

		// Safely decrease worker load
		if withinBounds {
			atomic.AddInt32(&s.workerLoads[workerID], -1)
		}
	}

	// After processing serverWorkerResetThreshold requests,
	// start a new worker and exit this one to reset the stack
	s.workerCreator()
}

// updatePoolMetrics updates the worker pool metrics for dynamic scaling
func (s *Server) updatePoolMetrics() {
	s.poolMetricsMu.Lock()
	defer s.poolMetricsMu.Unlock()

	// Update queue utilization
	queueLen := len(s.workerFuncChannel)
	queueCap := cap(s.workerFuncChannel)
	s.metrics.queueUsage = float64(queueLen) / float64(queueCap)

	// Update worker load metrics
	totalLoad := int32(0)
	activeWorkers := int32(0)

	for i, load := range s.workerLoads {
		if i < int(atomic.LoadInt32(&s.numWorkers)) {
			totalLoad += atomic.LoadInt32(&load)
			activeWorkers++
		}
	}

	if activeWorkers > 0 {
		s.metrics.idleWorkers = 1.0 - float64(totalLoad)/float64(activeWorkers)
	} else {
		s.metrics.idleWorkers = 1.0
	}

	// Update system resource utilization
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.metrics.cpuUsage = float64(m.Sys) / float64(runtime.NumCPU()*1024*1024)
	s.metrics.memoryUsage = float64(m.Alloc) / float64(m.Sys)
	s.metrics.lastAdjustTime = time.Now()
}

// monitorWorkerPool periodically monitors the worker pool and adjusts
// the number of workers based on the current load
func (s *Server) monitorWorkerPool() {
	// Use the configured adjustment interval
	ticker := time.NewTicker(s.opt.adjustInterval)
	defer ticker.Stop()

	var currentLoad float64
	var targetWorkers int32

	for range ticker.C {
		if s.isShutDown() {
			return
		}

		// update metrics
		s.updatePoolMetrics()

		// Get current load metrics
		s.poolMetricsMu.RLock()
		currentLoad = s.metrics.queueUsage
		idleWorkerRatio := s.metrics.idleWorkers
		s.poolMetricsMu.RUnlock()

		// Get the current number of workers
		currentWorkers := atomic.LoadInt32(&s.numWorkers)
		targetWorkers = currentWorkers

		if currentLoad > 0.8 || idleWorkerRatio < 0.2 {
			// Increase the number of worker threads under high load conditions
			if currentWorkers < int32(s.opt.maxWorkerPoolSize) {
				// Heavier loads, more worker threads
				targetWorkers = int32(float64(currentWorkers) * 1.2)
				if targetWorkers > int32(s.opt.maxWorkerPoolSize) {
					targetWorkers = int32(s.opt.maxWorkerPoolSize)
				}
			}
		} else if currentLoad < 0.2 && idleWorkerRatio > 0.8 {
			// Reduce the number of worker threads under low load conditions
			if currentWorkers > int32(s.opt.minWorkerPoolSize) {
				targetWorkers = int32(float64(currentWorkers) * 0.8)
				if targetWorkers < int32(s.opt.minWorkerPoolSize) {
					targetWorkers = int32(s.opt.minWorkerPoolSize)
				}
			}
		}

		// If adjustment is needed
		if targetWorkers != currentWorkers {
			adjustment := targetWorkers - currentWorkers
			if adjustment > 0 {
				// Add workers
				for i := int32(0); i < adjustment; i++ {
					s.workerCreator()
				}

				// Set the actual number of workers
				atomic.StoreInt32(&s.numWorkers, targetWorkers)

				zap.L().Info("Increased worker pool size",
					zap.Int32("from", currentWorkers),
					zap.Int32("to", targetWorkers),
					zap.Float64("queue_usage", currentLoad),
					zap.Float64("idle_workers", idleWorkerRatio))
			} else if adjustment < 0 {
				// Instead of actively shutting down reduced workers,
				// we let them exit naturally
				// Only update the target count, so that
				// new workers don't create replacement workers once the threshold is reached
				atomic.StoreInt32(&s.numWorkers, targetWorkers)

				zap.L().Info("Decreased worker pool size target",
					zap.Int32("from", currentWorkers),
					zap.Int32("to", targetWorkers),
					zap.Float64("queue_usage", currentLoad),
					zap.Float64("idle_workers", idleWorkerRatio))
			}
		}
	}
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

// Serve starts and listens to RPC requests.
// It is blocked until receiving connections from clients.
// This overrides the original Server's Serve method.
func (s *Server) Serve(network, address string) error {
	lis, err := s.makeListener(network, address)
	if err != nil {
		return err
	}

	return s.serveListener(lis)
}

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
// This overrides the original Server's serveConn method.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	// Most of the implementation remains the same as the original Server
	// with some adjustments to use the function-based worker pool

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
			zap.L().Error(fmt.Sprintf("[serving %s panic error: %s, stack:\n %s", conn.RemoteAddr(), err, buf))
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

		go func() {
			s.serveRequest(ctx, req, conn)
		}()
	}
}

func (s *Server) serveRequest(ctx context.Context, req *protocol.Message, conn net.Conn) {
	var err error
	// inject metadata to context
	ctx = metadata.NewInComingContext(ctx, req.Metadata)
	
	// Handle timeout from client
	if req.Metadata != nil {
		if timeoutStrs := req.Metadata.Get(protocol.TimeoutHeader); len(timeoutStrs) > 0 {
			if timeout, err := protocol.DecodeTimeout(timeoutStrs[0]); err == nil && timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
		}
	}
	
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
		return
	}

	// Use the function-based worker pool if enabled
	if s.opt.enableWorkerPool {
		// Create a function to process the request
		f := func() {
			s.processOneRequest(ctx, req, conn)
		}

		// Try to send to worker pool, fall back to direct execution if pool is full
		select {
		case s.workerFuncChannel <- f:
			// Request sent to the worker pool
		default:
			// Worker pool is full, check if we should scale up
			s.poolMetricsMu.RLock()
			queueUsage := s.metrics.queueUsage
			s.poolMetricsMu.RUnlock()

			// If the queue usage is high, trigger emergency scaling
			if queueUsage > 0.8 {
				s.quickScaleUp()
			}

			// Execute directly while scaling is in progress
			go f()
		}
	} else {
		go s.processOneRequest(ctx, req, conn)
	}
}

// quickScaleUp is an emergency scaling mechanism that rapidly increases
// the number of workers in response to a sudden spike in traffic
func (s *Server) quickScaleUp() {
	// Use atomic operations for thread safety
	currentWorkers := int(atomic.LoadInt32(&s.numWorkers))
	if currentWorkers >= s.opt.maxWorkerPoolSize {
		return
	}

	// Make sure we don't exceed the maximum pool size
	maxAllowed := s.opt.maxWorkerPoolSize

	// Calculate the target worker count with bounds checking
	targetWorkers := int(float64(currentWorkers) * 1.2)
	if targetWorkers > maxAllowed {
		targetWorkers = maxAllowed
	}

	// Calculate how many workers to add
	workersToAdd := targetWorkers - currentWorkers

	if workersToAdd <= 0 {
		return
	}

	// Use CAS (Compare-And-Swap) to ensure we don't create too many workers
	if !atomic.CompareAndSwapInt32(&s.numWorkers, int32(currentWorkers), int32(targetWorkers)) {
		// Someone else already adjusted the worker count, skip adjustment
		return
	}

	// Add the workers with a limit on concurrent additions
	batchSize := 10 // Add workers in batches to avoid overwhelming the system
	for added := 0; added < workersToAdd; {
		// Determine batch size for this iteration
		currentBatch := batchSize
		if workersToAdd-added < batchSize {
			currentBatch = workersToAdd - added
		}

		s.mu.Lock()
		for i := 0; i < currentBatch; i++ {
			s.workerCreator()
		}
		s.mu.Unlock()

		added += currentBatch

		// Small delay between batches to avoid overwhelming the system
		if added < workersToAdd {
			time.Sleep(time.Millisecond * 50)
		}
	}

	zap.L().Info("Emergency worker pool scaling completed",
		zap.Int("from", currentWorkers),
		zap.Int("to", targetWorkers),
		zap.Float64("queue_usage", s.metrics.queueUsage))
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

func chainServerMiddlewares(s *Server) {
	// Prepend opt.srvMiddleware to the chaining middlewares if it exists, since srvMiddleware will
	// be executed before any other chained middlewares.
	middlewares := s.opt.chainMiddlewares
	if s.opt.srvMiddleware != nil {
		middlewares = append([]ServerMiddleware{s.opt.srvMiddleware}, s.opt.chainMiddlewares...)
	}

	var chainedMws ServerMiddleware
	if len(middlewares) == 0 {
		chainedMws = nil
	} else if len(middlewares) == 1 {
		chainedMws = middlewares[0]
	} else {
		chainedMws = chainMiddlewares(middlewares)
	}

	s.opt.srvMiddleware = chainedMws
}

func chainMiddlewares(mws []ServerMiddleware) ServerMiddleware {
	return func(ctx context.Context, req any, info *ServerInfo, handler Handler) (resp any, err error) {
		return mws[0](ctx, req, info, getChainHandler(mws, 0, info, handler))
	}
}

func getChainHandler(mws []ServerMiddleware, pos int, info *ServerInfo, finalHandler Handler) Handler {
	if pos == len(mws)-1 {
		return finalHandler
	}
	return func(ctx context.Context, req any) (any, error) {
		return mws[pos+1](ctx, req, info, getChainHandler(mws, pos+1, info, finalHandler))
	}
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

		// Get or create mutex for this connection and lock it
		mu := s.getConnMutex(conn)
		mu.Lock()
		defer mu.Unlock()

		if s.opt.writeTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
		}
		conn.Write(msgBuffer.ReadOnlyData())
		msgBuffer.Free()
		return
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

		reply, err = md.Handler(srv.serviceImpl, ctx, df, s.opt.srvMiddleware)
		if err != nil {
			s.handleError(res, err)
		}
		if err != nil {
			zap.L().Error("zrpc: failed to handle request: ", zap.Error(err))
		}
	}

	s.sendResponse(conn, err, req, res, reply)
}

func (s *Server) sendResponse(conn net.Conn, originalErr error, req, res *protocol.Message, reply any) {
	var d mem.BufferSlice

	if reply != nil {
		var marshalErr error
		d, marshalErr = s.getCodec().Marshal(reply)
		if marshalErr != nil {
			if originalErr == nil {
				originalErr = marshalErr
				s.handleError(res, marshalErr)
			}
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

	// Get or create mutex for this connection and lock it
	mu := s.getConnMutex(conn)
	mu.Lock()
	defer mu.Unlock()

	if s.opt.writeTimeout != 0 {
		conn.SetWriteDeadline(time.Now().Add(s.opt.writeTimeout))
	}
	_, writeErr := conn.Write(msgBuffer.ReadOnlyData())
	msgBuffer.Free()
	if writeErr != nil {
		zap.L().Error("zrpc: failed to send response", zap.Error(writeErr))
	}
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

func (s *Server) isShutDown() bool {
	return atomic.LoadInt32(&s.inShutdown) == 1
}

func (s *Server) startShutdown() {
	if atomic.CompareAndSwapInt32(&s.inShutdown, 0, 1) {
		close(s.done)
	}
}

// Stop stops the server abruptly
func (s *Server) Stop() {
	s.stop(false)
}

// GracefulStop stops the server gracefully
func (s *Server) GracefulStop() {
	s.stop(true)
}

// stop is a helper method used by Stop and GracefulStop
func (s *Server) stop(graceful bool) {
	// Start the shutdown process
	s.startShutdown()

	// Close the listener first to stop accepting new connections
	s.mu.Lock()
	if s.lis != nil {
		s.lis.Close()
	}
	s.mu.Unlock()

	// Close the worker function channel if worker pool is enabled
	if s.opt.enableWorkerPool {
		// First drain the channel to avoid blocked workers
		go func() {
			// Set a timeout to ensure we don't block forever
			timeout := time.After(5 * time.Second)
			for {
				select {
				case <-timeout:
					// Timeout reached, proceed with closing
					s.workerFuncChannelClose()
					return
				default:
					// Try to drain the channel
					select {
					case <-s.workerFuncChannel:
						// Drained one item
					default:
						// Channel empty, close it
						s.workerFuncChannelClose()
						return
					}
				}
			}
		}()
	}

	// Wait for all serving goroutines to finish with timeout
	waitDone := make(chan struct{})
	go func() {
		s.serveWG.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// Successfully waited for all goroutines
	case <-time.After(30 * time.Second):
		zap.L().Warn("Timed out waiting for all goroutines to finish")
	}

	// If graceful shutdown, wait for all connections to close
	if graceful {
		connCloseDone := make(chan struct{})
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			for len(s.conns) > 0 {
				s.cv.Wait()
			}
			close(connCloseDone)
		}()

		select {
		case <-connCloseDone:
			// Successfully closed all connections
		case <-time.After(30 * time.Second):
			zap.L().Warn("Timed out waiting for all connections to close")
		}
	}

	s.mu.Lock()
	s.conns = nil
	s.mu.Unlock()
}

// removeConn removes a connection from the server's connection list
// This overrides the original Server's removeConn method with a safer implementation
func (s *Server) removeConn(conn net.Conn) {
	// Use a channel to signal when we're done with the mutex to avoid deadlocks
	done := make(chan struct{})

	go func() {
		// Try to acquire the lock with a timeout
		lockAcquired := make(chan struct{})

		go func() {
			s.mu.Lock()
			close(lockAcquired)
		}()

		select {
		case <-lockAcquired:
			// Successfully acquired the lock
			delete(s.conns, conn)
			s.cv.Broadcast()
			s.mu.Unlock()
		case <-time.After(5 * time.Second):
			// Timeout - log the issue but proceed with closing the connection
			zap.L().Warn("Timeout waiting for mutex in removeConn - possible deadlock averted")
		}

		// Clean up the connection mutex
		s.connMutexesMu.Lock()
		delete(s.connMutexes, conn)
		s.connMutexesMu.Unlock()

		// Always close the connection
		conn.Close()
		close(done)
	}()

	// Wait for connection removal to complete, but with a timeout
	select {
	case <-done:
		// Successfully removed
	case <-time.After(10 * time.Second):
		// If we're still stuck after 10 seconds, just return to avoid blocking forever
		zap.L().Error("Failed to remove connection after timeout - possible resource leak")
	}
}

// getConnMutex returns a mutex for a specific connection.
// If a mutex doesn't exist for this connection, it creates one.
func (s *Server) getConnMutex(conn net.Conn) *sync.Mutex {
	s.connMutexesMu.Lock()
	defer s.connMutexesMu.Unlock()

	mu, ok := s.connMutexes[conn]
	if !ok {
		mu = &sync.Mutex{}
		s.connMutexes[conn] = mu
	}
	return mu
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
