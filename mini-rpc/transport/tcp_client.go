// Kunhua Huang, 12/29/2025

package transport

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

type TCPClientTransport struct {
	addr string
	conn net.Conn
	mu   sync.Mutex
}

var _ ClientTransport = (*TCPClientTransport)(nil)

func NewTCPClientTransport(address string) *TCPClientTransport {
	return &TCPClientTransport{
		addr: address,
	}
}

func (c *TCPClientTransport) Connect() error {
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("connect to %s failed: %w", c.addr, err)
	}

	c.conn = conn
	fmt.Printf("connected to server %s\n", c.addr)
	return nil
}

func (c *TCPClientTransport) Send(data []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected to server, call Connect() first")
	}

	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	if _, err := c.conn.Write(lengthBuf); err != nil {
		return nil, fmt.Errorf("failed to send data length: %w", err)
	}

	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to send data: %w", err)
	}

	fmt.Printf("sent %d bytes to server\n", len(data))

	respLengthBuf := make([]byte, 4)
	if _, err := c.conn.Read(respLengthBuf); err != nil {
		return nil, fmt.Errorf("failed to read response length: %w", err)
	}

	respLength := binary.BigEndian.Uint32(respLengthBuf)

	if respLength > 10*1024*1024 {
		return nil, fmt.Errorf("response too large: %d bytes", respLength)
	}

	response := make([]byte, respLength)
	if _, err := io.ReadFull(c.conn, response); err != nil {
		return nil, fmt.Errorf("failed to read response data: %w", err)
	}

	fmt.Printf("received %d bytes from server\n", len(response))
	return response, nil
}

func (c *TCPClientTransport) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
		c.conn = nil
		fmt.Printf("connection to server %s closed\n", c.addr)
	}
	return nil
}
