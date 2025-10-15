package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// TcpRegistry is a simple TCP-based service registry.
type TcpRegistry struct {
	registry     *Registry
	listener     net.Listener
	connMap      sync.Map // Save the service for long connections, map[string]net.Conn
	keepaliveSec int      // Heartbeat timeout in seconds
	done         chan struct{}
}

type Request struct {
	Action      string            `json:"action"` // action: register, unregister, heartbeat, get, list
	ServiceName string            `json:"serviceName"`
	Addr        string            `json:"addr"`
	Metadata    map[string]string `json:"metadata"`
}

type Response struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Data    interface{} `json:"data"`
}

func NewTcpRegistry(addr string, keepaliveSec int) (*TcpRegistry, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("start tcp listen failed: %v", err)
	}

	if keepaliveSec <= 0 {
		keepaliveSec = 60
	}

	reg := NewRegistry(time.Duration(keepaliveSec) * time.Second)

	tcpReg := &TcpRegistry{
		registry:     reg,
		listener:     listener,
		keepaliveSec: keepaliveSec,
		done:         make(chan struct{}),
	}

	return tcpReg, nil
}

// Serve launches an enrollment center
func (r *TcpRegistry) Serve() {
	for {
		select {
		case <-r.done:
			return
		default:
			conn, err := r.listener.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				fmt.Printf("接受连接失败: %v\n", err)
				continue
			}

			go r.handleConnection(conn)
		}
	}
}

// handleConnection handle one tcp connection
func (r *TcpRegistry) handleConnection(conn net.Conn) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Duration(r.keepaliveSec) * time.Second))

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		if err != io.EOF {
			fmt.Printf("读取命令失败: %v\n", err)
		}
		return
	}

	var cmd Request
	if err := json.Unmarshal(buf[:n], &cmd); err != nil {
		r.sendResponse(conn, false, fmt.Sprintf("解析命令失败: %v", err), nil)
		return
	}

	switch cmd.Action {
	case "register":
		err := r.registry.Register(cmd.ServiceName, cmd.Addr, cmd.Metadata)
		if err != nil {
			r.sendResponse(conn, false, err.Error(), nil)
			return
		}
		// save long connections for heartbeat detection
		connKey := fmt.Sprintf("%s:%s", cmd.ServiceName, cmd.Addr)
		r.connMap.Store(connKey, conn)

		// initiate heartbeat detection
		go r.heartbeatChecker(cmd.ServiceName, cmd.Addr, conn)

		r.sendResponse(conn, true, "", nil)

	case "unregister":
		err := r.registry.Unregister(cmd.ServiceName, cmd.Addr)
		if err != nil {
			r.sendResponse(conn, false, err.Error(), nil)
			return
		}
		// remove long connections
		connKey := fmt.Sprintf("%s:%s", cmd.ServiceName, cmd.Addr)
		r.connMap.Delete(connKey)

		r.sendResponse(conn, true, "", nil)

	case "heartbeat":
		err := r.registry.Heartbeat(cmd.ServiceName, cmd.Addr)
		if err != nil {
			r.sendResponse(conn, false, err.Error(), nil)
			return
		}
		conn.SetDeadline(time.Now().Add(time.Duration(r.keepaliveSec) * time.Second))

		r.sendResponse(conn, true, "", nil)

	case "get":
		addrs, err := r.registry.GetService(cmd.ServiceName)
		if err != nil {
			r.sendResponse(conn, false, err.Error(), nil)
			return
		}
		r.sendResponse(conn, true, "", addrs)

	case "list":
		services := r.registry.GetAllServices()
		r.sendResponse(conn, true, "", services)

	default:
		r.sendResponse(conn, false, "未知命令", nil)
	}
}

// sendResponse sends a response
func (r *TcpRegistry) sendResponse(conn net.Conn, success bool, errMsg string, data interface{}) {
	resp := Response{
		Success: success,
		Error:   errMsg,
		Data:    data,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		fmt.Printf("序列化响应失败: %v\n", err)
		return
	}

	_, err = conn.Write(respData)
	if err != nil {
		fmt.Printf("发送响应失败: %v\n", err)
	}
}

// heartbeatChecker heartbeat detection
func (r *TcpRegistry) heartbeatChecker(serviceName, addr string, conn net.Conn) {
	connKey := fmt.Sprintf("%s:%s", serviceName, addr)
	ticker := time.NewTicker(time.Duration(r.keepaliveSec/3) * time.Second) // 检查频率为超时时间的1/3
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			// 检查服务是否超时
			if r.registry.IsExpired(serviceName, addr) {
				// 服务超时，注销服务
				r.registry.Unregister(serviceName, addr)
				r.connMap.Delete(connKey)
				fmt.Printf("服务 %s - %s 心跳超时，已注销\n", serviceName, addr)
				return
			}
		}
	}
}

// Close closes the TCP registry
func (r *TcpRegistry) Close() error {
	close(r.done)

	// 关闭所有连接
	r.connMap.Range(func(key, value interface{}) bool {
		conn := value.(net.Conn)
		conn.Close()
		return true
	})

	return r.listener.Close()
}

// GetRegistry to get the internal Registry.
func (r *TcpRegistry) GetRegistry() *Registry {
	return r.registry
}
