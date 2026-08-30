// Kunhua Huang 2026

package server

import (
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/logger"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/ratelimiter"
	"github.com/ecstasoy/RPCinGo/pkg/registry"
	"github.com/ecstasoy/RPCinGo/pkg/transport"
)

type serverOptions struct {
	address        string
	codecType      protocol.CodecType
	compressType   protocol.CompressType
	readTimeout    time.Duration
	writeTimeout   time.Duration
	handlerTimeout time.Duration
	maxConcurrent  int
	workerPoolSize int

	// transportOptions are raw transport-level options forwarded verbatim to
	// the underlying transport after the named server options are translated.
	transportOptions []transport.ServerOption

	interceptors []interceptor.Interceptor
	logger       logger.Logger

	// Registry options
	serviceName       string
	serviceVersion    string
	registry          registry.Registry
	enableRegistry    bool
	heartbeatInterval time.Duration
}

func defaultServerOptions() *serverOptions {
	return &serverOptions{
		address:        ":8080",
		codecType:      protocol.CodecTypeJSON,
		compressType:   protocol.CompressTypeNone,
		readTimeout:    10 * time.Second,
		writeTimeout:   10 * time.Second,
		handlerTimeout: 0, // unlimited by default
		maxConcurrent:  0,
		workerPoolSize: 8,

		enableRegistry:    false,
		heartbeatInterval: 5 * time.Second,
	}
}

// Option mutates server configuration before NewServer constructs the server.
type Option func(*serverOptions)

// WithAddress sets the listen address used by Start.
func WithAddress(addr string) Option {
	return func(o *serverOptions) {
		o.address = addr
	}
}

// WithCodec selects the request/response body codec and transport compression
// used by the server.
func WithCodec(codec protocol.CodecType, compress protocol.CompressType) Option {
	return func(o *serverOptions) {
		o.codecType = codec
		o.compressType = compress
	}
}

// WithTimeout configures transport-level read and write deadlines.
func WithTimeout(read, write time.Duration) Option {
	return func(o *serverOptions) {
		o.readTimeout = read
		o.writeTimeout = write
	}
}

// WithHandlerTimeout limits how long a single request handler may run before
// its context is canceled; zero (the default) disables the limit. This closes
// the long-standing gap where HandlerTimeout — the server's only real
// per-request budget — could only be set by reaching into the transport
// package directly.
func WithHandlerTimeout(timeout time.Duration) Option {
	return func(o *serverOptions) {
		o.handlerTimeout = timeout
	}
}

// WithTransportOptions forwards raw transport-level server options to the
// underlying transport. It gives access to every transport knob (buffer sizes,
// max request body size, and any future option) without a per-knob server
// wrapper, so the two option surfaces compose instead of one silently dropping
// what the other defines. Forwarded options are applied after the translated
// named options, so they take precedence.
func WithTransportOptions(opts ...transport.ServerOption) Option {
	return func(o *serverOptions) {
		o.transportOptions = append(o.transportOptions, opts...)
	}
}

// WithConcurrency configures the maximum number of concurrent request handlers
// and the worker-pool size used by the transport.
func WithConcurrency(maxConcurrent, workerPoolSize int) Option {
	return func(o *serverOptions) {
		o.maxConcurrent = maxConcurrent
		o.workerPoolSize = workerPoolSize
	}
}

// WithRegistry enables service registration, deregistration, and heartbeat
// management against reg for the supplied service identity.
func WithRegistry(serviceName, version string, reg registry.Registry) Option {
	return func(o *serverOptions) {
		o.serviceName = serviceName
		o.serviceVersion = version
		o.registry = reg
		o.enableRegistry = true
	}
}

// WithHeartbeatInterval sets the heartbeat period used when registry
// integration is enabled.
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(o *serverOptions) {
		o.heartbeatInterval = interval
	}
}

// WithInterceptors appends server-side interceptors executed in registration
// order around every request.
func WithInterceptors(interceptors ...interceptor.Interceptor) Option {
	return func(o *serverOptions) {
		o.interceptors = append(o.interceptors, interceptors...)
	}
}

// WithRateLimit prepends a rate-limiting interceptor so overload is rejected
// before later interceptors and handlers run.
func WithRateLimit(limiter ratelimiter.RateLimiter) Option {
	return func(o *serverOptions) {
		o.interceptors = append([]interceptor.Interceptor{interceptor.RateLimit(limiter)}, o.interceptors...)
	}
}

// WithLogger sets the logger used by server internals.
func WithLogger(l logger.Logger) Option {
	return func(o *serverOptions) {
		o.logger = l
	}
}
