package client

import (
	"context"
	"fmt"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/transport/tcp"
)

type Client struct {
	pool     *tcp.ConnectionPool
	codec    protocol.CodecType
	compress protocol.CompressType
}

func NewClient(address string, opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, o := range opts {
		o(options)
	}

	pool, err := tcp.NewConnectionPool(
		address,
		tcp.WithPoolSize(options.maxConnections, options.minConnections),
		tcp.WithPoolCodec(options.codecType, options.compressType),
		tcp.WithIdleTimeout(options.idleTimeout),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		pool:     pool,
		codec:    options.codecType,
		compress: options.compressType,
	}, nil
}

func (c *Client) Call(ctx context.Context, service, method string, args any) (*protocol.Response, error) {
	if c.fixedMode {
		return c.callFixed(ctx, service, method, args)
	}

	if c.breakerOn {
		cb := c.getCircuitBreaker(service)
		return cb.CallResponse(ctx, func() (*protocol.Response, error) {
			return c.callWithDiscovery(ctx, service, method, args)
		})
	}

	return c.callWithDiscovery(ctx, service, method, args)
}

func (c *Client) callFixed(ctx context.Context, service, method string, args any) (*protocol.Response, error) {
	conn, err := c.fixedPool.GetWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	defer conn.Release()

	req := protocol.NewRequest(service, method, args)

	conn, err := c.pool.GetWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	defer conn.Release()

	resp, err := conn.Client.SendRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("remote error: %s", resp.Error)
	}

	return resp.Data, nil
}

func (c *Client) Close() error {
	return c.pool.Close()
}
