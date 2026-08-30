package client

import (
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/ratelimiter"

	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/loadbalancer"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry"
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

// Option mutates client configuration before a Client is constructed.
type Option func(*clientOptions)

// WithCodec selects the request/response body codec and transport compression
// used by the client.
func WithCodec(codec protocol.CodecType, compress protocol.CompressType) Option {
	return func(o *clientOptions) {
		o.codecType = codec
		o.compressType = compress
	}
}

// WithPoolSize configures the connection pool size used by fixed-address
// clients created with NewClient.
func WithPoolSize(max, min int) Option {
	return func(o *clientOptions) {
		o.maxConnections = max
		o.minConnections = min
	}
}

// WithTimeout applies a per-call timeout around Client.Call.
func WithTimeout(timeout time.Duration) Option {
	return func(o *clientOptions) {
		o.callTimeout = timeout
	}
}

// WithDiscovery supplies the discovery backend required by
// NewDiscoveryClient.
func WithDiscovery(discovery registry.Discovery) Option {
	return func(o *clientOptions) {
		o.discovery = discovery
	}
}

// WithLoadBalancer sets the load balancer used to pick a service instance in
// discovery mode.
func WithLoadBalancer(lb loadbalancer.LoadBalancer) Option {
	return func(o *clientOptions) {
		o.loadBalancer = lb
	}
}

// WithWatch enables or disables background discovery watches in discovery
// mode. It has no effect for fixed-address clients created with NewClient.
func WithWatch(enable bool) Option {
	return func(o *clientOptions) {
		o.enableWatch = enable
	}
}

// WithCircuitBreaker enables or disables circuit-breaker protection in
// discovery mode.
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

// WithRateLimit prepends a client-side rate-limiting interceptor so rejected
// calls fail before network I/O begins.
func WithRateLimit(limiter ratelimiter.RateLimiter) Option {
	return func(o *clientOptions) {
		o.interceptors = append([]interceptor.Interceptor{interceptor.RateLimit(limiter)}, o.interceptors...)
	}
}
