/*
{"level":"info","ts":1742786111.9787652,"caller":"client/client.go:47","msg":"Starting benchmark","concurrency":100,"requests_per_client":10000,"total_requests":1000000,"pool_size":200,"conn_timeout":5,"req_timeout":10}
{"level":"info","ts":1742786111.978817,"caller":"client/client.go:73","msg":"Warming up..."}
{"level":"info","ts":1742786111.9818418,"caller":"client/client.go:80","msg":"Warm-up complete"}
{"level":"info","ts":1742786163.5559824,"caller":"client/client.go:165","msg":"took 51574 ms for 1000000 requests"}
{"level":"info","ts":1742786163.749295,"caller":"client/client.go:190","msg":"sent     requests    : 1000000\n"}
{"level":"info","ts":1742786163.7493677,"caller":"client/client.go:191","msg":"received requests    : 1000000\n"}
{"level":"info","ts":1742786163.7493734,"caller":"client/client.go:192","msg":"received requests_OK : 1000000\n"}
{"level":"info","ts":1742786163.749377,"caller":"client/client.go:193","msg":"error    requests    : 0\n"}
{"level":"info","ts":1742786163.7493825,"caller":"client/client.go:194","msg":"success  rate        : 100.00%\n"}
{"level":"info","ts":1742786163.7493997,"caller":"client/client.go:195","msg":"throughput  (TPS)    : 19389\n"}
{"level":"info","ts":1742786163.7494059,"caller":"client/client.go:196","msg":"mean: 5153191 ns, median: 5105133 ns, max: 106724618 ns, min: 28564 ns, p99: 19323752 ns\n"}
{"level":"info","ts":1742786163.749412,"caller":"client/client.go:197","msg":"mean: 5 ms, median: 5 ms, max: 106 ms, min: 0 ms, p99: 19 ms\n"}

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
	"sync"
	"sync/atomic"
	"time"

	"github.com/montanaflynn/stats"
	"go.uber.org/zap"

	"github.com/crazyfrankie/zrpc"
	"github.com/crazyfrankie/zrpc/benchmark/bench"
)

var (
	concurrency  = flag.Int("concurrency", 1, "use for client concurrency")
	total        = flag.Int("total", 0, "total requests for client")
	host         = flag.String("host", "localhost:8082", "server ip and port")
	poolSize     = flag.Int("pool_size", 200, "connection pool size")
	connTimeout  = flag.Duration("conn_timeout", 5*time.Second, "connection timeout")
	reqTimeout   = flag.Duration("req_timeout", 10*time.Second, "request timeout")
	retryCount   = flag.Int("retry", 3, "retry count for failed requests")
	waitRetry    = flag.Duration("wait_retry", 100*time.Millisecond, "wait time between retries")
	reportErrors = flag.Bool("report_errors", false, "report detailed error information")
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()        // 确保日志输出
	zap.ReplaceGlobals(logger) // 替换全局日志实例

	flag.Parse()

	n := *concurrency
	m := *total / n

	req := prepareArgs()

	var wg sync.WaitGroup
	wg.Add(n * m)

	// 添加日志信息
	logger.Info("Starting benchmark",
		zap.Int("concurrency", n),
		zap.Int("requests_per_client", m),
		zap.Int("total_requests", n*m),
		zap.Int("pool_size", *poolSize),
		zap.Duration("conn_timeout", *connTimeout),
		zap.Duration("req_timeout", *reqTimeout))

	clientOptions := []zrpc.ClientOption{
		zrpc.DialWithMaxPoolSize(*poolSize),
		zrpc.DialWithConnectTimeout(*connTimeout),
		// TCP保持活动选项，避免连接被关闭
		zrpc.DialWithTCPKeepAlive(30 * time.Second),
		// 请求空闲超时
		zrpc.DialWithIdleTimeout(30 * time.Second),
	}

	client, err := zrpc.NewClient(*host, clientOptions...)
	if err != nil {
		logger.Fatal("Failed to create client", zap.Error(err))
	}

	cc := bench.NewHelloServiceClient(client)

	// warm up
	logger.Info("Warming up...")
	for i := 0; i < 10; i++ {
		_, err := cc.Say(context.Background(), req)
		if err != nil {
			logger.Warn("Warm-up request failed", zap.Error(err))
		}
	}
	logger.Info("Warm-up complete")

	var startWg sync.WaitGroup
	startWg.Add(n)

	var trans uint64
	var transOK uint64
	var errorCount uint64

	// 用于统计不同类型的错误
	errorTypes := make(map[string]uint64)
	var errorMutex sync.Mutex

	d := make([][]int64, n, n)

	totalT := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		dt := make([]int64, 0, m)
		d = append(d, dt)

		go func(clientID int) {
			startWg.Done()
			startWg.Wait()

			for j := 0; j < m; j++ {
				t := time.Now().UnixNano()

				// 实现带重试的请求
				var res *bench.BenchmarkMessage
				var err error
				success := false

				for retry := 0; retry <= *retryCount; retry++ {
					// 创建新的context以便每次重试有完整的超时时间
					ctx, cancel := context.WithTimeout(context.Background(), *reqTimeout)
					res, err = cc.Say(ctx, req)

					if err == nil && res != nil && res.Field1 == "OK" {
						success = true
						cancel()
						break
					}

					cancel() // 取消当前的context

					if *reportErrors && err != nil {
						errType := fmt.Sprintf("%T", err)
						errorMutex.Lock()
						errorTypes[errType]++
						errorMutex.Unlock()

						if retry == *retryCount {
							logger.Debug("Request failed after retries",
								zap.Int("client_id", clientID),
								zap.Int("request", j),
								zap.Error(err),
								zap.Int("retries", retry))
						}
					}

					// 最后一次重试后不用等待
					if retry < *retryCount {
						time.Sleep(*waitRetry)
					}
				}

				t = time.Now().UnixNano() - t

				d[clientID] = append(d[clientID], t)

				atomic.AddUint64(&trans, 1)
				if success {
					atomic.AddUint64(&transOK, 1)
				} else {
					atomic.AddUint64(&errorCount, 1)
				}

				wg.Done()
			}
		}(i)
	}

	wg.Wait()
	totalT = time.Now().UnixNano() - totalT
	totalT = totalT / 1000000
	logger.Info(fmt.Sprintf("took %d ms for %d requests", totalT, n*m))

	// 如果有错误，输出错误类型统计
	if *reportErrors && len(errorTypes) > 0 {
		logger.Info("Error type statistics:")
		for errType, count := range errorTypes {
			logger.Info(fmt.Sprintf("  %s: %d", errType, count))
		}
	}

	totalD := make([]int64, 0, n*m)
	for _, k := range d {
		totalD = append(totalD, k...)
	}
	totalD2 := make([]float64, 0, n*m)
	for _, k := range totalD {
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
