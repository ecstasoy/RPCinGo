package client

import (
	"RPCinGo/pkg/ratelimiter"
	"time"

	"RPCinGo/pkg/interceptor"
	"RPCinGo/pkg/loadbalancer"
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/registry"
)

type clientOptions struct {
	codecType      protocol.CodecType
	compressType   protocol.CompressType
	maxConnections int
	minConnections int
	idleTimeout    time.Duration
	callTimeout    time.Duration

	discovery            registry.Discovery
	loadBalancer         loadbalancer.LoadBalancer
	enableWatch          bool
	enableCircuitBreaker bool

	interceptors  []interceptor.Interceptor
	maxRetries    int
	retryInterval time.Duration
}

func defaultOptions() *clientOptions {
	return &clientOptions{
		codecType:      protocol.CodecTypeJSON,
		compressType:   protocol.CompressTypeNone,
		maxConnections: 100,
		minConnections: 10,
		idleTimeout:    90 * time.Second,
		callTimeout:    5 * time.Second,

		loadBalancer:         loadbalancer.NewRoundRobin(),
		enableWatch:          true,
		enableCircuitBreaker: true,
	}
}

type Option func(*clientOptions)

func WithCodec(codec protocol.CodecType, compress protocol.CompressType) Option {
	return func(o *clientOptions) {
		o.codecType = codec
		o.compressType = compress
	}
}

func WithPoolSize(max, min int) Option {
	return func(o *clientOptions) {
		o.maxConnections = max
		o.minConnections = min
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *clientOptions) {
		o.callTimeout = timeout
	}
}

func WithDiscovery(discovery registry.Discovery) Option {
	return func(o *clientOptions) {
		o.discovery = discovery
	}
}

func WithLoadBalancer(lb loadbalancer.LoadBalancer) Option {
	return func(o *clientOptions) {
		o.loadBalancer = lb
	}
}

func WithWatch(enable bool) Option {
	return func(o *clientOptions) {
		o.enableWatch = enable
	}
}

func WithCircuitBreaker(enable bool) Option {
	return func(o *clientOptions) {
		o.enableCircuitBreaker = enable
	}
}

// WithClientInterceptors adds client-side interceptors executed around every Call.
// Multiple calls to WithClientInterceptors append to the existing list.
// Execution order matches registration order (first registered = outermost).
func WithClientInterceptors(interceptors ...interceptor.Interceptor) Option {
	return func(o *clientOptions) {
		o.interceptors = append(o.interceptors, interceptors...)
	}
}

// WithRetry enables automatic retry on transient failures.
// maxRetries is the number of additional attempts after the first (e.g. 2 → up to 3 total).
// A Retry interceptor is prepended to the chain so it wraps all other interceptors.
func WithRetry(maxRetries int, retryInterval time.Duration) Option {
	return func(o *clientOptions) {
		o.maxRetries = maxRetries
		o.retryInterval = retryInterval
	}
}

func WithRateLimit(limiter ratelimiter.RateLimiter) Option {
	return func(o *clientOptions) {
		o.interceptors = append([]interceptor.Interceptor{interceptor.RateLimit(limiter)}, o.interceptors...)
	}
}
