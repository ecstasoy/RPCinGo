package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ecstasoy/RPCinGo/pkg/circuitbreaker"

	"github.com/ecstasoy/RPCinGo/pkg/codec"
	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/pool"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry"

	"google.golang.org/protobuf/proto"
)

// Client is the high-level RPC client used for either fixed-address or
// discovery-based calls. The two modes differ only in how a connection is
// acquired, which lives behind the connSource seam; Call itself has one path.
type Client struct {
	opts *clientOptions

	// source is the seam that hides fixed-vs-discovery connection acquisition.
	source connSource

	breakers  map[string]*circuitbreaker.CircuitBreaker
	breakerMu sync.RWMutex
	breakerOn bool

	codec        codec.Codec
	interceptors []interceptor.Interceptor
}

// NewClient constructs a fixed-address client backed by one connection pool for
// address. Discovery-only options are rejected so misconfiguration fails loudly
// instead of being silently ignored.
func NewClient(address string, opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, o := range opts {
		o(options)
	}

	if options.discovery != nil {
		return nil, fmt.Errorf("NewClient is for fixed-address clients; use NewDiscoveryClient when supplying WithDiscovery")
	}

	p, err := pool.NewConnectionPool(
		address,
		pool.WithPoolSize(options.maxConnections, options.minConnections),
		pool.WithPoolCodec(options.codecType, options.compressType),
		pool.WithIdleTimeout(options.idleTimeout),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		opts:         options,
		source:       &fixedSource{pool: p},
		codec:        codec.Get(options.codecType),
		interceptors: buildInterceptors(options),
		// breakerOn stays false: a fixed-address client has no circuit breaker.
	}, nil
}

// NewDiscoveryClient constructs a discovery-based client that resolves
// instances via WithDiscovery and load balances requests across them. The
// configured pool size is honored per endpoint (no longer hardcoded).
func NewDiscoveryClient(opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, o := range opts {
		o(options)
	}

	if options.discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}

	poolManager := pool.NewPoolManager(
		options.codecType,
		options.compressType,
		pool.WithManagerPoolSize(options.maxConnections, options.minConnections),
	)

	return &Client{
		opts: options,
		source: &discoverySource{
			poolManager:   poolManager,
			discovery:     options.discovery,
			loadBalancer:  options.loadBalancer,
			enableWatch:   options.enableWatch,
			instanceCache: make(map[string][]*registry.ServiceInstance),
			watchers:      make(map[string]registry.Watcher),
		},
		breakers:     make(map[string]*circuitbreaker.CircuitBreaker),
		breakerOn:    options.enableCircuitBreaker,
		codec:        codec.Get(options.codecType),
		interceptors: buildInterceptors(options),
	}, nil
}

// Use appends client-side interceptors that wrap every Call.
// Interceptors added via Use run after those set at construction time (e.g. Retry).
func (c *Client) Use(interceptors ...interceptor.Interceptor) {
	c.interceptors = append(c.interceptors, interceptors...)
}

// Call invokes service.method with args and returns the decoded RPC response.
func (c *Client) Call(ctx context.Context, service, method string, args any) (*protocol.Response, error) {
	if c.opts.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.opts.callTimeout)
		defer cancel()
	}

	req := protocol.NewRequest(service, method, args)

	if key, ok := hashKeyFromContext(ctx); ok {
		req.SetMetadata(protocol.MetaKeyHashKey, key)
	}

	invoker := func(ctx context.Context, req *protocol.Request) (any, error) {
		if c.breakerOn {
			cb := c.getCircuitBreaker(req.Service)
			return cb.CallResponse(ctx, func() (*protocol.Response, error) {
				return c.callOnce(ctx, req)
			})
		}
		return c.callOnce(ctx, req)
	}

	chain := interceptor.NewChain(c.interceptors...)
	result, err := chain.Intercept(ctx, req, invoker)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*protocol.Response), nil
}

// callOnce performs one attempt: acquire a connection from the source, send the
// request, and translate a protocol error into a Go error. It is the single
// connection-using path shared by both modes.
func (c *Client) callOnce(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	conn, err := c.source.acquire(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := conn.Client.SendRequest(ctx, req)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send: %w", err)
	}
	conn.Release()

	if resp.IsError() {
		return nil, unmapError(resp)
	}

	return resp, nil
}

// buildInterceptors assembles the final interceptor slice for a new Client.
// If WithRetry was set, a Retry interceptor is prepended (outermost) so it
// retries the full chain on transient failures.
func buildInterceptors(opts *clientOptions) []interceptor.Interceptor {
	var chain []interceptor.Interceptor
	if opts.maxRetries > 0 {
		chain = append(chain, interceptor.Retry(opts.maxRetries, opts.retryInterval))
	}
	chain = append(chain, opts.interceptors...)
	return chain
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

// CallTyped invokes service.method with a protobuf request and unmarshals the
// typed response payload into resp.
func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) (*protocol.Response, error) {
	respData, err := c.Call(ctx, service, method, req)
	if err != nil {
		return nil, err
	}

	if respData.IsError() {
		return respData, fmt.Errorf("rpc error %s", respData.Error.Error())
	}

	if respData.Data == nil {
		return respData, nil
	}

	dataBytes, ok := respData.Data.([]byte)
	if !ok {
		dataBytes, err = json.Marshal(respData.Data)
		if err != nil {
			return respData, fmt.Errorf("marshal response data: %w", err)
		}
	}

	switch respData.DataCodec {
	case protocol.PayloadCodecProtobuf:
		return respData, proto.Unmarshal(dataBytes, resp)
	case protocol.PayloadCodecJSON:
		return respData, json.Unmarshal(dataBytes, resp)
	case protocol.PayloadCodecRaw:
		return respData, fmt.Errorf("cannot unmarshal raw bytes into typed response")
	default:
		if err := proto.Unmarshal(dataBytes, resp); err == nil {
			return respData, nil
		}
		return respData, json.Unmarshal(dataBytes, resp)
	}
}

// Close releases all client resources, including pools and discovery watchers.
func (c *Client) Close() error {
	return c.source.Close()
}
