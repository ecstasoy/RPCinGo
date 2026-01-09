//Kunhua Huang 2026

package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/transport"
)

type Client struct {
	address   string
	opts      *transport.ClientOptions
	Codec     *ProtocolCodec
	conn      net.Conn
	connected bool
	mu        sync.RWMutex // protects connected and conn
	sendMu    sync.Mutex   // protects send operations
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

	return nil
}

func (c *Client) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	reqData, err := c.Codec.EncodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	respData, err := c.Send(ctx, reqData)
	if err != nil {
		return nil, err
	}

	resp, err := c.Codec.DecodeResponse(respData)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return resp, nil
}

func (c *Client) Send(ctx context.Context, data []byte) ([]byte, error) {
	c.mu.RLock()
	if !c.connected || c.conn == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected, call Dial() first")
	}
	conn := c.conn
	c.mu.RUnlock()

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return nil, fmt.Errorf("set write deadline failed: %w", err)
		}
	} else {
		if err := conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); err != nil {
			return nil, fmt.Errorf("set write deadline failed: %w", err)
		}
	}

	if err := writeFull(conn, data); err != nil {
		return nil, fmt.Errorf("write to connection failed: %w", err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, fmt.Errorf("set read deadline failed: %w", err)
		}
	} else {
		if err := conn.SetReadDeadline(time.Now().Add(c.opts.ReadTimeout)); err != nil {
			return nil, fmt.Errorf("set read deadline failed: %w", err)
		}
	}

	header, respBody, err := c.Codec.DecodeFromReader(conn)
	if err != nil {
		return nil, fmt.Errorf("read response from connection failed: %w", err)
	}

	if header.MsgType != protocol.MsgTypeResponse {
		return nil, fmt.Errorf("expected response message type, got: %s", header.MsgType)
	}

	// Clear deadlines
	err = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return nil, fmt.Errorf("set write deadline failed: %w", err)
	}
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, fmt.Errorf("set read deadline failed: %w", err)
	}

	return respBody, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("close connection failed: %w", err)
		}
		c.conn = nil
	}

	c.connected = false

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

func (c *Client) SendWithRetry(ctx context.Context, data []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := c.Send(ctx, data)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if attempt == c.opts.MaxRetries {
			break
		}

		select {
		case <-time.After(c.opts.RetryInterval):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("send failed after %d retries: %w", c.opts.MaxRetries, lastErr)
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
