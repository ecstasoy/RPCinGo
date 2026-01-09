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
	resp, err := conn.Client.SendRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	if resp.IsError() {
		return nil, unmapError(resp)
	}

	return resp, nil
}

func (c *Client) callWithDiscovery(ctx context.Context, service, method string, args any) (*protocol.Response, error) {
	instances, err := c.getInstances(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("get instances: %w", err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no available instances for %s", service)
	}

	instance, err := c.loadBalancer.Pick(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("pick instance: %w", err)
	}

	conn, err := c.poolManager.GetConnection(ctx, instance.Endpoint())
	if err != nil {
		return nil, fmt.Errorf("get connection to %s: %w", instance.Endpoint(), err)
	}
	defer conn.Release()

	req := protocol.NewRequest(service, method, args)
	resp, err := conn.Client.SendRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	if resp.IsError() {
		return nil, unmapError(resp)
	}

	return resp, nil
}

func (c *Client) getInstances(ctx context.Context, service string) ([]*registry.ServiceInstance, error) {
	c.cacheMu.RLock()
	cached, ok := c.instanceCache[service]
	c.cacheMu.RUnlock()

	if ok && len(cached) > 0 {
		return cached, nil
	}

	if c.discovery == nil {
		return nil, fmt.Errorf("no discovery configured")
	}

	instances, err := c.discovery.GetInstances(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("discovery get instances: %w", err)
	}

	c.cacheMu.Lock()
	c.instanceCache[service] = instances
	c.cacheMu.Unlock()

	if c.opts.enableWatch {
		go c.watchService(service)
	}

	return instances, nil
}

func (c *Client) watchService(service string) {
	c.watchMu.Lock()
	if _, watching := c.watchers[service]; watching {
		c.watchMu.Unlock()
		return
	}

	watcher, err := c.discovery.Watch(context.Background(), service)
	if err != nil {
		c.watchMu.Unlock()
		return
	}

	c.watchers[service] = watcher
	c.watchMu.Unlock()

	for {
		event, err := watcher.Next()
		if err != nil {
			return
		}

		c.handleWatchEvent(service, event)
	}
}

func (c *Client) handleWatchEvent(service string, event *registry.Event) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	instances := c.instanceCache[service]

	switch event.Type {
	case registry.EventTypeAdd:
		instances = append(instances, event.Instance)
	case registry.EventTypeDelete:
		filtered := make([]*registry.ServiceInstance, 0, len(instances))
		for _, inst := range instances {
			if inst.ID != event.Instance.ID {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered

		c.poolManager.RemovePool(event.Instance.Endpoint())
	case registry.EventTypeUpdate:
		for i, inst := range instances {
			if inst.ID == event.Instance.ID {
				instances[i] = event.Instance
				break
			}
		}
	}

	c.instanceCache[service] = instances
}

func (c *Client) getCircuitBreaker(service string) *circuitbreaker.CircuitBreaker {
	c.breakerMu.RLock()
	cb, exists := c.breakers[service]
	c.breakerMu.RUnlock()

	if exists {
		return cb
	}

	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()

	cb, exists = c.breakers[service]
	if exists {
		return cb
	}

	cb = circuitbreaker.New(circuitbreaker.DefaultConfig())
	c.breakers[service] = cb

	return cb
}

func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) error {
	respData, err := c.Call(ctx, service, method, req)
	if err != nil {
		return err
	}

	if respData.IsError() {
		return fmt.Errorf("rpc error %s", respData.Error.Error())
	}

	if respData.Data == nil {
		return nil
	}

	dataBytes, ok := respData.Data.([]byte)
	if !ok {
		dataBytes, err = json.Marshal(respData.Data)
		if err != nil {
			return fmt.Errorf("marshal response data: %w", err)
		}
	}

	switch respData.DataCodec {
	case protocol.PayloadCodecProtobuf:
		return proto.Unmarshal(dataBytes, resp)
	case protocol.PayloadCodecJSON:
		return json.Unmarshal(dataBytes, resp)
	case protocol.PayloadCodecRaw:
		return fmt.Errorf("cannot unmarshal raw bytes into typed response")
	default:
		if err := proto.Unmarshal(dataBytes, resp); err == nil {
			return nil
		}
		return json.Unmarshal(dataBytes, resp)
	}
}

func (c *Client) Close() error {
	if c.fixedMode {
		return c.fixedPool.Close()
	}

	c.watchMu.Lock()
	for _, watcher := range c.watchers {
		watcher.Stop()
	}
	c.watchMu.Unlock()

	return c.poolManager.Close()
}
