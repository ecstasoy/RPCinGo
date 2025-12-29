// Kunhua Huang, 12/29/2025

package client

import (
	"fmt"
	"mini-rpc/codec"
	"mini-rpc/protocol"
	"mini-rpc/transport"
	"sync"
)

type Client struct {
	addr      string
	transport transport.ClientTransport
	codec     codec.Codec
	mu        sync.Mutex
	connected bool
}

func NewClient(addr string) *Client {
	return &Client{
		addr:      addr,
		transport: transport.NewTCPClientTransport(addr),
		codec:     codec.GetCodec(codec.JSONCodecType),
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	if err := c.transport.Connect(); err != nil {
		return fmt.Errorf("transport connect failed: %w", err)
	}

	c.connected = true
	return nil
}

func (c *Client) Call(service, method string, args []interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := protocol.NewRequest(service, method, args)

	fmt.Printf("[Client] Calling %s.%s with args: %v\n", service, method, args)

	reqData, err := c.codec.Encode(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	fmt.Printf("[Client] Encoded request data: %v\n", reqData)

	respData, err := c.transport.Send(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	fmt.Printf("[Client] Received response data: %v\n", respData)

	var resp protocol.Response
	if err := c.codec.Decode(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("[Client] Decoded response: %+v\n", resp)

	if resp.ID != req.ID {
		return nil, fmt.Errorf("mismatched response ID: got %d, want %d", resp.ID, req.ID)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("server error: %s", resp.Error)
	}

	fmt.Printf("[Client] Call successful, result: %v\n", resp.Data)
	return resp.Data, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	return c.transport.Close()
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}
