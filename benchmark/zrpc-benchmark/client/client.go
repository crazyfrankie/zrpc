/*
client: poolSize: 2000
server: workerNum: runtime.NumCPU()*6
{"level":"info","ts":1742802185.7984157,"caller":"client/client.go:98","msg":"Starting benchmark","concurrency":100,"requests_per_client":10000,"total_requests":1000000,"pool_size":2000,"conn_timeout":3,"req_timeout":5,"warmup":1000,"batch_size":50,"cpu_cores":16}
{"level":"info","ts":1742802185.7984653,"caller":"client/client.go:127","msg":"Warming up..."}
{"level":"info","ts":1742802185.8915365,"caller":"client/client.go:129","msg":"Warm-up complete"}
{"level":"info","ts":1742802202.2898283,"caller":"client/client.go:240","msg":"took 16335 ms for 1000000 requests"}
{"level":"info","ts":1742802202.4650855,"caller":"client/client.go:261","msg":"sent     requests    : 1000000\n"}
{"level":"info","ts":1742802202.465144,"caller":"client/client.go:262","msg":"received requests    : 1000000\n"}
{"level":"info","ts":1742802202.4651492,"caller":"client/client.go:263","msg":"received requests_OK : 1000000\n"}
{"level":"info","ts":1742802202.465153,"caller":"client/client.go:264","msg":"error    requests    : 0\n"}
{"level":"info","ts":1742802202.4651577,"caller":"client/client.go:265","msg":"success  rate        : 100.00%\n"}
{"level":"info","ts":1742802202.465162,"caller":"client/client.go:266","msg":"throughput  (TPS)    : 61218\n"}
{"level":"info","ts":1742802202.4651673,"caller":"client/client.go:267","msg":"mean: 72434956 ns, median: 71343850 ns, max: 573502937 ns, min: 26220 ns, p99: 536236587 ns\n"}
{"level":"info","ts":1742802202.4651723,"caller":"client/client.go:268","msg":"mean: 72 ms, median: 71 ms, max: 573 ms, min: 0 ms, p99: 536 ms\n"}

client: poolSize: 500
server: workerNum: runtime.NumCPU()*4
{"level":"info","ts":1742797565.6278405,"caller":"client/client.go:99","msg":"Starting benchmark","concurrency":100,"requests_per_client":10000,"total_requests":1000000,"pool_size":500,"conn_timeout":3,"req_timeout":5,"warmup":1000,"batch_size":50,"cpu_cores":16}
{"level":"info","ts":1742797565.6278949,"caller":"client/client.go:128","msg":"Warming up..."}
{"level":"info","ts":1742797565.7819161,"caller":"client/client.go:130","msg":"Warm-up complete"}
{"level":"info","ts":1742797590.9405153,"caller":"client/client.go:241","msg":"took 25158 ms for 1000000 requests"}
{"level":"info","ts":1742797591.0976772,"caller":"client/client.go:262","msg":"sent     requests    : 1000000\n"}
{"level":"info","ts":1742797591.0977495,"caller":"client/client.go:263","msg":"received requests    : 1000000\n"}
{"level":"info","ts":1742797591.0977714,"caller":"client/client.go:264","msg":"received requests_OK : 1000000\n"}
{"level":"info","ts":1742797591.0977845,"caller":"client/client.go:265","msg":"error    requests    : 0\n"}
{"level":"info","ts":1742797591.0977902,"caller":"client/client.go:266","msg":"success  rate        : 100.00%\n"}
{"level":"info","ts":1742797591.097803,"caller":"client/client.go:267","msg":"throughput  (TPS)    : 39748\n"}
{"level":"info","ts":1742797591.097824,"caller":"client/client.go:268","msg":"mean: 109822742 ns, median: 122570408 ns, max: 322326657 ns, min: 34526 ns, p99: 195403420 ns\n"}
{"level":"info","ts":1742797591.0978434,"caller":"client/client.go:269","msg":"mean: 109 ms, median: 122 ms, max: 322 ms, min: 0 ms, p99: 195 ms\n"}
rpcx:
2025/03/24 11:18:09 rpcx_single_client.go:34: INFO : Servers: [0xc00019ac20]

2025/03/24 11:18:09 rpcx_single_client.go:36: INFO : concurrency: 100
requests per client: 10000

2025/03/24 11:18:09 rpcx_single_client.go:45: INFO : message size: 581 bytes

2025/03/24 11:18:27 rpcx_single_client.go:106: INFO : took 18294 ms for 1000000 requests
2025/03/24 11:18:27 rpcx_single_client.go:123: INFO : sent     requests    : 1000000
2025/03/24 11:18:27 rpcx_single_client.go:124: INFO : received requests    : 1000000
2025/03/24 11:18:27 rpcx_single_client.go:125: INFO : received requests_OK : 1000000
2025/03/24 11:18:27 rpcx_single_client.go:126: INFO : throughput  (TPS)    : 54662
2025/03/24 11:18:27 rpcx_single_client.go:127: INFO : mean: 1826766 ns, median: 1725462 ns, max: 11319208 ns, min: 44891 ns, p99: 5790435 ns
2025/03/24 11:18:27 rpcx_single_client.go:128: INFO : mean: 1 ms, median: 1 ms, max: 11 ms, min: 0 ms, p99: 5 ms
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/montanaflynn/stats"
	"go.uber.org/zap"

	"github.com/crazyfrankie/zrpc"
	"github.com/crazyfrankie/zrpc/benchmark/bench"
)

var (
	concurrency  = flag.Int("concurrency", runtime.NumCPU()*10, "client concurrency")
	total        = flag.Int("total", 0, "total requests for client")
	poolSize     = flag.Int("pool_size", 500, "connection pool size")
	connTimeout  = flag.Duration("conn_timeout", 3*time.Second, "connection timeout")
	reqTimeout   = flag.Duration("req_timeout", 5*time.Second, "request timeout")
	retryCount   = flag.Int("retry", 2, "retry count for failed requests")
	waitRetry    = flag.Duration("wait_retry", 10*time.Millisecond, "wait time between retries")
	reportErrors = flag.Bool("report_errors", false, "report detailed error information")
	warmupCount  = flag.Int("warmup", 1000, "number of warmup requests")
	useSharedReq = flag.Bool("shared_req", true, "use shared request object")
	batchSize    = flag.Int("batch", 50, "number of requests per batch")
)

// 使用共享请求
var sharedRequest *bench.BenchmarkMessage

// 使用对象池减少GC压力
var messagePool = sync.Pool{
	New: func() interface{} {
		return &bench.BenchmarkMessage{}
	},
}

func main() {
	// 设置CPU核心数
	runtime.GOMAXPROCS(runtime.NumCPU())

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	flag.Parse()

	n := *concurrency
	m := *total / n
	if m == 0 && *total > 0 {
		m = 1
	}

	// 准备共享请求对象
	sharedRequest = prepareArgs()

	var wg sync.WaitGroup
	wg.Add(n * m)

	// 添加日志信息
	logger.Info("Starting benchmark",
		zap.Int("concurrency", n),
		zap.Int("requests_per_client", m),
		zap.Int("total_requests", n*m),
		zap.Int("pool_size", *poolSize),
		zap.Duration("conn_timeout", *connTimeout),
		zap.Duration("req_timeout", *reqTimeout),
		zap.Int("warmup", *warmupCount),
		zap.Int("batch_size", *batchSize),
		zap.Int("cpu_cores", runtime.NumCPU()),
	)

	// 添加客户端选项
	clientOptions := []zrpc.ClientOption{
		zrpc.DialWithMaxPoolSize(*poolSize),
		zrpc.DialWithConnectTimeout(*connTimeout),
		zrpc.DialWithTCPKeepAlive(30 * time.Second),
		zrpc.DialWithIdleTimeout(30 * time.Second),
		zrpc.DialWithRegistryAddress("localhost:8084"),
	}

	client, err := zrpc.NewClient("registry:///bench", clientOptions...)
	if err != nil {
		logger.Fatal("Failed to create client", zap.Error(err))
	}
	defer client.Close()

	cc := bench.NewHelloServiceClient(client)

	// 预热连接和服务器
	logger.Info("Warming up...")
	warmupClient(cc, *warmupCount)
	logger.Info("Warm-up complete")

	var startWg sync.WaitGroup
	startWg.Add(n)

	var trans uint64
	var transOK uint64
	var errorCount uint64

	// 用于统计不同类型的错误
	errorTypes := make(map[string]uint64)
	var errorMutex sync.Mutex

	// 使用更高效的时间测量
	d := make([]int64, 0, n*m)
	var dMutex sync.Mutex

	totalT := time.Now().UnixNano()

	// 创建批处理工作线程
	for i := 0; i < n; i++ {
		go func(clientID int) {
			startWg.Done()
			startWg.Wait()

			// 每个批次里处理batchSize个请求
			for j := 0; j < m; j += *batchSize {
				batchCount := *batchSize
				if j+batchCount > m {
					batchCount = m - j
				}

				// 每个批次使用一个context和多个goroutine
				ctx, cancel := context.WithTimeout(context.Background(), *reqTimeout)
				var batchWg sync.WaitGroup
				batchWg.Add(batchCount)

				for k := 0; k < batchCount; k++ {
					go func() {
						defer batchWg.Done()

						t := time.Now().UnixNano()

						// 获取请求对象 - 共享或从池中获取
						var req *bench.BenchmarkMessage
						if *useSharedReq {
							req = sharedRequest
						} else {
							req = getRequestFromPool()
							defer putRequestToPool(req)
						}

						// 发送请求并重试
						var res *bench.BenchmarkMessage
						var err error
						success := false

						for retry := 0; retry <= *retryCount; retry++ {
							res, err = cc.Say(ctx, req)

							if err == nil && res != nil && res.Field1 == "OK" {
								success = true
								break
							}

							if ctx.Err() != nil || retry == *retryCount {
								break
							}

							if *reportErrors && err != nil {
								errType := fmt.Sprintf("%T", err)
								errorMutex.Lock()
								errorTypes[errType]++
								errorMutex.Unlock()
							}

							// 最后一次重试后不等待
							if retry < *retryCount {
								time.Sleep(*waitRetry)
							}
						}

						t = time.Now().UnixNano() - t

						// 安全地添加延迟记录
						dMutex.Lock()
						d = append(d, t)
						dMutex.Unlock()

						atomic.AddUint64(&trans, 1)
						if success {
							atomic.AddUint64(&transOK, 1)
						} else {
							atomic.AddUint64(&errorCount, 1)
						}

						// 减少主等待组计数
						wg.Done()
					}()
				}

				// 等待批次完成
				batchWg.Wait()
				cancel()
			}
		}(i)
	}

	wg.Wait()
	totalT = time.Now().UnixNano() - totalT
	totalT = totalT / 1000000
	logger.Info(fmt.Sprintf("took %d ms for %d requests", totalT, n*m))

	// 输出错误类型统计
	if *reportErrors && len(errorTypes) > 0 {
		logger.Info("Error type statistics:")
		for errType, count := range errorTypes {
			logger.Info(fmt.Sprintf("  %s: %d", errType, count))
		}
	}

	totalD2 := make([]float64, 0, len(d))
	for _, k := range d {
		totalD2 = append(totalD2, float64(k))
	}

	mean, _ := stats.Mean(totalD2)
	median, _ := stats.Median(totalD2)
	max, _ := stats.Max(totalD2)
	min, _ := stats.Min(totalD2)
	p99, _ := stats.Percentile(totalD2, 99.9)

	logger.Info(fmt.Sprintf("sent     requests    : %d\n", n*m))
	logger.Info(fmt.Sprintf("received requests    : %d\n", atomic.LoadUint64(&trans)))
	logger.Info(fmt.Sprintf("received requests_OK : %d\n", atomic.LoadUint64(&transOK)))
	logger.Info(fmt.Sprintf("error    requests    : %d\n", atomic.LoadUint64(&errorCount)))
	logger.Info(fmt.Sprintf("success  rate        : %.2f%%\n", float64(atomic.LoadUint64(&transOK))*100/float64(n*m)))
	logger.Info(fmt.Sprintf("throughput  (TPS)    : %d\n", int64(n*m)*1000/totalT))
	logger.Info(fmt.Sprintf("mean: %.f ns, median: %.f ns, max: %.f ns, min: %.f ns, p99: %.f ns\n", mean, median, max, min, p99))
	logger.Info(fmt.Sprintf("mean: %d ms, median: %d ms, max: %d ms, min: %d ms, p99: %d ms\n", int64(mean/1000000), int64(median/1000000), int64(max/1000000), int64(min/1000000), int64(p99/1000000)))
}

// 预热客户端和服务器
func warmupClient(client bench.HelloServiceClient, count int) {
	ctx := context.Background()

	var wg sync.WaitGroup
	threads := runtime.NumCPU()
	perThread := count / threads

	if perThread < 1 {
		perThread = 1
		threads = count
	}

	wg.Add(threads)

	for i := 0; i < threads; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perThread; j++ {
				_, _ = client.Say(ctx, sharedRequest)
			}
		}()
	}

	wg.Wait()
}

// 从池中获取请求对象
func getRequestFromPool() *bench.BenchmarkMessage {
	msg := messagePool.Get().(*bench.BenchmarkMessage)
	*msg = *sharedRequest
	return msg
}

func putRequestToPool(msg *bench.BenchmarkMessage) {
	messagePool.Put(msg)
}

func prepareArgs() *bench.BenchmarkMessage {
	var i int32 = 100000
	var s = "许多往事在眼前一幕一幕，变的那麼模糊"
	var b = true

	var args bench.BenchmarkMessage

	v := reflect.ValueOf(&args).Elem()
	num := v.NumField()
	for k := 0; k < num; k++ {
		field := v.Field(k)

		if !field.CanSet() {
			continue
		}

		switch field.Kind() {
		case reflect.Int, reflect.Int32, reflect.Int64:
			field.SetInt(int64(i))
		case reflect.Bool:
			field.SetBool(b)
		case reflect.String:
			field.SetString(s)
		case reflect.Slice: // 处理 repeated 字段
			if field.Type().Elem().Kind() == reflect.Uint64 {
				field.Set(reflect.ValueOf([]uint64{1, 2, 3, 4, 5}))
			}
		}
	}

	return &args
}
