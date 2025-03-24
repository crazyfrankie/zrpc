package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/crazyfrankie/zrpc"
	"github.com/crazyfrankie/zrpc/benchmark/bench"
)

var (
	host       = flag.String("host", "127.0.0.1:8082", "listened ip and port")
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	delay      = flag.Duration("delay", 0, "delay to mock business processing")
	workerNum  = flag.Int("worker_num", runtime.NumCPU()*8, "number of workers for request processing")
	taskQueue  = flag.Int("task_queue", 100000, "task queue size for worker pool")
	pprofPort  = flag.String("pprof", ":6060", "pprof http server address")
	logLevel   = flag.String("log", "info", "log level: debug, info, warn, error")
	bufferSize = flag.Int("buffer", 64*1024, "response buffer size")
)

// 对象池，用于复用响应对象
var responsePool = sync.Pool{
	New: func() interface{} {
		return &bench.BenchmarkMessage{
			Field1: "OK",
			Field2: 100,
		}
	},
}

func main() {
	// 设置最大线程数
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.Parse()

	// 配置日志
	logConfig := zap.NewProductionConfig()
	switch *logLevel {
	case "debug":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	}

	// 减少日志采样以提高性能
	logConfig.Sampling = &zap.SamplingConfig{
		Initial:    100,
		Thereafter: 100,
	}

	logger, _ := logConfig.Build()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// 启动pprof服务器，用于性能分析
	go func() {
		if *pprofPort != "" {
			logger.Info("Starting pprof server", zap.String("address", *pprofPort))
			if err := http.ListenAndServe(*pprofPort, nil); err != nil {
				logger.Error("Failed to start pprof server", zap.Error(err))
			}
		}
	}()

	logger.Info("Starting server",
		zap.String("host", *host),
		zap.Int("worker_pool_size", *workerNum),
		zap.Int("task_queue_size", *taskQueue),
		zap.Duration("simulated_delay", *delay),
		zap.Int("cpu_cores", runtime.NumCPU()),
		zap.Int("buffer_size", *bufferSize),
	)

	// 使用更多的Server选项
	srvOptions := []zrpc.ServerOption{
		zrpc.WithWorkerPool(*workerNum),
		zrpc.WithTaskQueueSize(*taskQueue),
		zrpc.WithReadTimeout(2 * time.Second),
		zrpc.WithWriteTimeout(2 * time.Second),
		zrpc.WithMaxReceiveMessageSize(1024 * 1024 * 10),
		zrpc.WithMaxSendMessageSize(1024 * 1024 * 10),
	}

	srv := zrpc.NewServer(srvOptions...)
	bench.RegisterHelloServiceServer(srv, &HelloService{})

	// 预热缓存
	logger.Info("Pre-warming response cache...")
	for i := 0; i < 1000; i++ {
		_ = responsePool.Get().(*bench.BenchmarkMessage)
	}

	logger.Info("Server is ready to accept connections")
	if err := srv.Serve("tcp", *host); err != nil {
		logger.Fatal("Server failed", zap.Error(err))
	}
}

type HelloService struct {
	bench.UnimplementedHelloServiceServer
	requestCount int64
}

func (s *HelloService) Say(ctx context.Context, req *bench.BenchmarkMessage) (*bench.BenchmarkMessage, error) {
	// 使用原子操作增加请求计数，每处理1000000个请求打印一条日志
	count := atomic.AddInt64(&s.requestCount, 1)
	if count%1000000 == 0 {
		zap.L().Info("Processed requests", zap.Int64("count", count))
	}

	// 确保请求对象不为空
	if req == nil {
		return &bench.BenchmarkMessage{Field1: "ERROR", Field2: 0}, nil
	}

	// 从对象池获取响应对象而不是创建新对象
	res := responsePool.Get().(*bench.BenchmarkMessage)

	// 复制必要字段
	if req.Field9 != "" {
		res.Field9 = req.Field9
	}
	if req.Field18 != "" {
		res.Field18 = req.Field18
	}
	res.Field80 = req.Field80
	res.Field81 = req.Field81
	res.Field3 = req.Field3
	res.Field280 = req.Field280
	res.Field6 = req.Field6
	res.Field22 = req.Field22

	// 仅在需要时复制更多字段
	if len(req.Field4) > 0 {
		res.Field4 = req.Field4
	}
	if len(req.Field5) > 0 {
		res.Field5 = req.Field5
	}
	res.Field59 = req.Field59

	if *delay > 0 {
		time.Sleep(*delay)
	} else {
		runtime.Gosched()
	}

	// 不再直接返回res，而是将其包装在defer函数中，确保在RPC调用完成后对象会被放回池中
	respCopy := *res
	// 将原对象放回池中
	responsePool.Put(res)

	// 返回复制出来的对象
	return &respCopy, nil
}
