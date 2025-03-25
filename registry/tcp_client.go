package registry

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// TcpClient is a simple TCP client to communicate with the TcpRegistry server.
type TcpClient struct {
	registryAddr string
	serviceName  string
	serviceAddr  string
	metadata     map[string]string
	conn         net.Conn
	mu           sync.Mutex
	done         chan struct{}
}

func NewTcpClient(registryAddr string) *TcpClient {
	return &TcpClient{
		registryAddr: registryAddr,
		done:         make(chan struct{}),
	}
}

// connect connects to the registration center
func (c *TcpClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.Dial("tcp", c.registryAddr)
	if err != nil {
		return fmt.Errorf("连接注册中心失败: %v", err)
	}

	c.conn = conn
	return nil
}

// close closes the connection
func (c *TcpClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// sendRequest Sends a command and receives a response.
func (c *TcpClient) sendRequest(cmd Request) (*Response, error) {
	if err := c.connect(); err != nil {
		return nil, err
	}
	defer c.close()

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("序列化命令失败: %v", err)
	}

	_, err = c.conn.Write(data)
	if err != nil {
		return nil, fmt.Errorf("发送命令失败: %v", err)
	}

	buf := make([]byte, 4096)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("接收响应失败: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("命令执行失败: %s", resp.Error)
	}

	return &resp, nil
}

// Register Register Services
func (c *TcpClient) Register(serviceName, serviceAddr string, metadata map[string]string) error {
	cmd := Request{
		Action:      "register",
		ServiceName: serviceName,
		Addr:        serviceAddr,
		Metadata:    metadata,
	}

	_, err := c.sendRequest(cmd)
	if err != nil {
		return err
	}

	c.serviceName = serviceName
	c.serviceAddr = serviceAddr
	c.metadata = metadata

	// Start heartbeat
	go c.heartbeat()

	return nil
}

// Unregister logout service
func (c *TcpClient) Unregister() error {
	if c.serviceName == "" || c.serviceAddr == "" {
		return fmt.Errorf("register service yet")
	}

	cmd := Request{
		Action:      "unregister",
		ServiceName: c.serviceName,
		Addr:        c.serviceAddr,
	}

	_, err := c.sendRequest(cmd)
	if err != nil {
		return err
	}

	close(c.done)

	return nil
}

// heartbeat sends a heartbeat
func (c *TcpClient) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			cmd := Request{
				Action:      "heartbeat",
				ServiceName: c.serviceName,
				Addr:        c.serviceAddr,
			}

			_, err := c.sendRequest(cmd)
			if err != nil {
				fmt.Printf("send heart beat: %v\n", err)
			}
		}
	}
}

// GetService returns a list of services
func (c *TcpClient) GetService(serviceName string) ([]string, error) {
	cmd := Request{
		Action:      "get",
		ServiceName: serviceName,
	}

	resp, err := c.sendRequest(cmd)
	if err != nil {
		return nil, err
	}

	addrs, ok := resp.Data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unknown response type")
	}

	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addrStr, ok := addr.(string)
		if !ok {
			continue
		}
		result = append(result, addrStr)
	}

	return result, nil
}

// ListServices returns all services
func (c *TcpClient) ListServices() ([]string, error) {
	cmd := Request{
		Action: "list",
	}

	resp, err := c.sendRequest(cmd)
	if err != nil {
		return nil, err
	}

	services, ok := resp.Data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unknown response type")
	}

	result := make([]string, 0, len(services))
	for _, svc := range services {
		svcStr, ok := svc.(string)
		if !ok {
			continue
		}
		result = append(result, svcStr)
	}

	return result, nil
}
