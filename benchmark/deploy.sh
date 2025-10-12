#!/bin/bash

# ZRPC Benchmark 部署脚本
# 用于启动 registry、server、client 进行性能测试

set -e

# 默认配置
REGISTRY_PORT=8084
SERVER_PORT=8082
PPROF_PORT=6060
PROMETHEUS_PORT=9091
LOG_LEVEL="info"
WORKER_NUM=$(nproc)
POOL_SIZE=1000
CONCURRENCY=100
TOTAL_REQUESTS=1000000
SERVER_COUNT=1
CLIENT_COUNT=1

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

# 清理函数
cleanup() {
    log "正在清理进程..."
    pkill -f "./registry" 2>/dev/null || true
    pkill -f "./server" 2>/dev/null || true
    pkill -f "./client" 2>/dev/null || true
    sleep 2
}

# 检查端口是否被占用
check_port() {
    if lsof -Pi :$1 -sTCP:LISTEN -t >/dev/null 2>&1; then
        error "端口 $1 已被占用"
        return 1
    fi
    return 0
}

# 构建组件
build_components() {
    log "构建 benchmark 组件..."
    
    cd zrpc-benchmark/registry
    go build -o registry registry.go
    
    cd ../server
    go build -o server server.go
    
    cd ../client
    go build -o client client.go
    
    cd ../..
    log "构建完成"
}

# 启动 registry
start_registry() {
    log "启动 Registry (端口: $REGISTRY_PORT)..."
    check_port $REGISTRY_PORT
    
    cd zrpc-benchmark/registry
    nohup ./registry > registry.log 2>&1 &
    REGISTRY_PID=$!
    cd ../..
    
    sleep 2
    if kill -0 $REGISTRY_PID 2>/dev/null; then
        log "Registry 启动成功 (PID: $REGISTRY_PID)"
    else
        error "Registry 启动失败"
        exit 1
    fi
}

# 启动多个 server
start_servers() {
    log "启动 $SERVER_COUNT 个 Server 实例..."
    
    for i in $(seq 1 $SERVER_COUNT); do
        local port=$((SERVER_PORT + i - 1))
        local pprof_port=$((PPROF_PORT + i - 1))
        local prom_port=$((PROMETHEUS_PORT + i - 1))
        
        check_port $port
        check_port $pprof_port
        check_port $prom_port
        
        log "启动 Server-$i (端口: $port)"
        cd zrpc-benchmark/server
        nohup ./server \
            -host="0.0.0.0:$port" \
            -worker_num=$WORKER_NUM \
            -log=$LOG_LEVEL \
            -pprof=":$pprof_port" \
            -prom=":$prom_port" > server-$i.log 2>&1 &
        eval "SERVER_PID_$i=$!"
        cd ../..
        
        sleep 1
    done
    
    sleep 2
    log "所有 Server 启动完成"
}

# 运行多个 client 测试
run_clients() {
    log "启动 $CLIENT_COUNT 个 Client 实例..."
    
    local requests_per_client=$((TOTAL_REQUESTS / CLIENT_COUNT))
    
    cd zrpc-benchmark/client
    for i in $(seq 1 $CLIENT_COUNT); do
        log "启动 Client-$i (请求数: $requests_per_client)"
        nohup ./client \
            -concurrency=$CONCURRENCY \
            -total=$requests_per_client \
            -pool_size=$POOL_SIZE \
            -conn_timeout=3s \
            -req_timeout=5s \
            -warmup=1000 \
            -batch=50 > client-$i.log 2>&1 &
        eval "CLIENT_PID_$i=$!"
    done
    cd ../..
    
    # 等待所有client完成
    for i in $(seq 1 $CLIENT_COUNT); do
        eval "wait \$CLIENT_PID_$i"
        log "Client-$i 完成"
    done
}

# 显示帮助
show_help() {
    cat << EOF
ZRPC Benchmark 部署脚本

用法: $0 [选项] [命令]

命令:
    build       构建所有组件
    registry    启动 Registry
    server      启动 Server  
    client      运行 Client 测试
    all         启动 Registry + Server + Client (默认)
    stop        停止所有服务
    clean       清理构建文件

选项:
    -s, --servers NUM       Server 实例数量 (默认: 1)
    --clients NUM           Client 实例数量 (默认: 1)
    -w, --workers NUM       每个Server的Worker数量 (默认: CPU核心数)
    -p, --pool-size NUM     连接池大小 (默认: 1000)
    -c, --concurrency NUM   每个Client的并发数 (默认: 100)
    -t, --total NUM         总请求数 (默认: 1000000)
    -l, --log-level LEVEL   日志级别 (默认: info)
    --registry-port PORT    Registry 端口 (默认: 8084)
    --server-port PORT      Server 端口 (默认: 8082)
    --pprof-port PORT       pprof 端口 (默认: 6060)
    --prom-port PORT        Prometheus 端口 (默认: 9091)
    -h, --help              显示帮助

示例:
    $0                                    # 运行完整测试 (1个registry, 1个server, 1个client)
    $0 -s 3 --clients 2                 # 3个server, 2个client
    $0 -c 200 -t 2000000 -s 2           # 每client 200并发，200万总请求，2个server
    $0 --workers 16 --pool-size 2000    # 每server 16个worker，2000连接池
    $0 server                            # 只启动server
    $0 stop                              # 停止所有服务
EOF
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--servers)
            SERVER_COUNT="$2"
            shift 2
            ;;
        --clients)
            CLIENT_COUNT="$2"
            shift 2
            ;;
        -w|--workers)
            WORKER_NUM="$2"
            shift 2
            ;;
        -p|--pool-size)
            POOL_SIZE="$2"
            shift 2
            ;;
        -c|--concurrency)
            CONCURRENCY="$2"
            shift 2
            ;;
        -t|--total)
            TOTAL_REQUESTS="$2"
            shift 2
            ;;
        -l|--log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        --registry-port)
            REGISTRY_PORT="$2"
            shift 2
            ;;
        --server-port)
            SERVER_PORT="$2"
            shift 2
            ;;
        --pprof-port)
            PPROF_PORT="$2"
            shift 2
            ;;
        --prom-port)
            PROMETHEUS_PORT="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        build|registry|server|client|all|stop|clean)
            COMMAND="$1"
            shift
            ;;
        *)
            error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 默认命令
COMMAND=${COMMAND:-all}

# 进入benchmark目录
cd "$(dirname "$0")"

case $COMMAND in
    build)
        build_components
        ;;
    registry)
        cleanup
        build_components
        start_registry
        log "Registry 运行中，按 Ctrl+C 停止"
        wait
        ;;
    server)
        cleanup
        build_components
        start_servers
        log "Server(s) 运行中，按 Ctrl+C 停止"
        wait
        ;;
    client)
        build_components
        run_clients
        ;;
    all)
        cleanup
        build_components
        start_registry
        start_servers
        sleep 2
        run_clients
        cleanup
        ;;
    stop)
        cleanup
        log "所有服务已停止"
        ;;
    clean)
        cleanup
        rm -f zrpc-benchmark/registry/registry
        rm -f zrpc-benchmark/server/server
        rm -f zrpc-benchmark/client/client
        rm -f zrpc-benchmark/*/*.log
        log "清理完成"
        ;;
    *)
        error "未知命令: $COMMAND"
        show_help
        exit 1
        ;;
esac
