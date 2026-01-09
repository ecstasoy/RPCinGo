package client

import (
	"RPCinGo/pkg/circuitbreaker"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"RPCinGo/pkg/codec"
	"RPCinGo/pkg/loadbalancer"
	"RPCinGo/pkg/pool"
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/registry"

	"google.golang.org/protobuf/proto"
)

type Client struct {
	opts *clientOptions

	poolManager  *pool.PoolManager
	discovery    registry.Discovery
	loadBalancer loadbalancer.LoadBalancer

	instanceCache map[string][]*registry.ServiceInstance
	cacheMu       sync.RWMutex

	watchers map[string]registry.Watcher
	watchMu  sync.Mutex

	// Fixed mode (single instance)
	fixedPool *pool.ConnectionPool
	fixedMode bool

	breakers  map[string]*circuitbreaker.CircuitBreaker
	breakerMu sync.RWMutex
	breakerOn bool

	codec codec.Codec
}

func NewClient(address string, opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, o := range opts {
		o(options)
	}

	pool, err := pool.NewConnectionPool(
		address,
		pool.WithPoolSize(options.maxConnections, options.minConnections),
		pool.WithPoolCodec(options.codecType, options.compressType),
		pool.WithIdleTimeout(options.idleTimeout),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		opts:      options,
		fixedPool: pool,
		fixedMode: true,
		codec:     codec.Get(options.codecType),
	}, nil
}

func NewDiscoveryClient(opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, o := range opts {
		o(options)
	}

	if options.discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}

	return &Client{
		opts:          options,
		poolManager:   pool.NewPoolManager(options.codecType, options.compressType),
		discovery:     options.discovery,
		loadBalancer:  options.loadBalancer,
		instanceCache: make(map[string][]*registry.ServiceInstance),
		watchers:      make(map[string]registry.Watcher),
		breakers:      make(map[string]*circuitbreaker.CircuitBreaker),
		breakerOn:     options.enableCircuitBreaker,
		fixedMode:     false,
		codec:         codec.Get(options.codecType),
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
