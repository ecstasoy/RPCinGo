//Kunhua Huang 2026

package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"

	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/transport"
)

// pendingCall represents a pending request awaiting a response or error.
// done is a channel used to signal the completion of the pending call.
// respBody contains the decompressed and partially decoded response body bytes.
// err holds any error encountered during request processing.
type pendingCall struct {
	done     chan struct{}
	respBody []byte // decompressed response body bytes, decoded by the caller
	err      error
}

// Client manages the network connection, request encoding, decoding, and concurrency for a client in an RPC framework.
type Client struct {
	address   string
	opts      *transport.ClientOptions
	Codec     *ProtocolCodec
	conn      net.Conn
	connected bool
	mu        sync.RWMutex // protects connected and conn

	writeMu   sync.Mutex              // 保护并发写
	pendingMu sync.Mutex              // 保护 pending map
	pending   map[uint64]*pendingCall // requestID -> caller
	closeCh   chan struct{}           // read loop 退出信号
	closeOnce sync.Once
}

var _ transport.ClientTransport = (*Client)(nil)

func NewClient(
	address string,
	codecType protocol.CodecType,
	compressType protocol.CompressType,
	options ...transport.ClientOption,
) *Client {
	opts := transport.DefaultClientOptions()

	for _, o := range options {
		o(opts)
	}

	return &Client{
		address: address,
		opts:    opts,
		Codec:   NewProtocolCodec(codecType, compressType),
	}
}

// Dial establishes a TCP connection to the specified address and initializes the client state for further interactions.
func (c *Client) Dial(ctx context.Context, address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected to: %s", c.conn.RemoteAddr().String())
	}

	addr := address
	if addr == "" {
		addr = c.address
	}

	dialer := &net.Dialer{
		Timeout:   c.opts.DialTimeout,
		KeepAlive: c.opts.KeepAlivePeriod,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial tcp %s failed: %w", addr, err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if c.opts.KeepAlive {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				_ = conn.Close()
				return fmt.Errorf("set keep-alive failed: %w", err)
			}

			if err := tcpConn.SetKeepAlivePeriod(c.opts.KeepAlivePeriod); err != nil {
				_ = conn.Close()
				return fmt.Errorf("set keep-alive period failed: %w", err)
			}
		}

		if err := tcpConn.SetNoDelay(true); err != nil {
			_ = conn.Close()
			return fmt.Errorf("set no delay failed: %w", err)
		}
	}

	c.conn = conn
	c.connected = true
	c.address = addr
	c.pending = make(map[uint64]*pendingCall)
	c.closeCh = make(chan struct{})
	go c.readLoop(conn)

	return nil
}

// SendRequest sends a request to the server, waits for the response, and returns the decoded result or an error.
func (c *Client) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	call := &pendingCall{done: make(chan struct{})}

	c.pendingMu.Lock()
	c.pending[req.ID] = call
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := c.Codec.WriteRequest(c.conn, req)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, req.ID)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write request failed: %w", err)
	}

	select {
	case <-call.done:
		if call.err != nil {
			return nil, call.err
		}
		return c.Codec.DecodeResponse(call.respBody)
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, req.ID)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, fmt.Errorf("connection closed")
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil
	}
	conn := c.conn
	c.conn = nil
	c.connected = false
	c.mu.Unlock()

	// Closing the conn causes readLoop's DecodeFromReader to return an error,
	// which triggers closeWithError. We also call it directly to unblock any
	// pending callers in case readLoop hasn't noticed yet.
	var closeErr error
	if conn != nil {
		closeErr = conn.Close()
	}
	c.closeWithError(fmt.Errorf("connection closed"))

	if closeErr != nil {
		return fmt.Errorf("close connection failed: %w", closeErr)
	}
	return nil
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) LocalAddr() net.Addr {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn != nil {
		return c.conn.LocalAddr()
	}

	return nil
}

func (c *Client) RemoteAddr() net.Addr {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn != nil {
		return c.conn.RemoteAddr()
	}

	return nil
}

// readLoop receives conn as a parameter to avoid racing with Close() which sets
// c.conn = nil. The goroutine holds its own reference to the connection.
func (c *Client) readLoop(conn net.Conn) {
	defer c.closeWithError(fmt.Errorf("connection closed"))

	for {
		header, bodyBytes, err := c.Codec.DecodeFromReader(conn)
		if err != nil {
			return
		}

		c.pendingMu.Lock()
		call, ok := c.pending[header.RequestID]
		if ok {
			delete(c.pending, header.RequestID)
		}
		c.pendingMu.Unlock()

		if ok {
			call.respBody = bodyBytes
			close(call.done)
		}
	}
}

func (c *Client) closeWithError(err error) {
	c.closeOnce.Do(func() {
		close(c.closeCh)

		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		c.pendingMu.Lock()
		for id, call := range c.pending {
			call.err = err
			close(call.done)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	})
}

func writeFull(w net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
