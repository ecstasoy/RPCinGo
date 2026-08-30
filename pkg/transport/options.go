package transport

import (
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/logger"
)

// ClientOptions groups transport-level client tuning knobs. Concrete transport
// implementations may honor only the subset they support.
type ClientOptions struct {
	DialTimeout         time.Duration
	KeepAlive           bool
	KeepAlivePeriod     time.Duration
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	MaxIdleConnDuration time.Duration
	ReadBufferSize      int
	WriteBufferSize     int
	MaxRetries          int
	RetryInterval       time.Duration
}

// DefaultClientOptions returns the default transport client settings.
func DefaultClientOptions() *ClientOptions {
	return &ClientOptions{
		DialTimeout:     5 * time.Second,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,

		ReadTimeout:         10 * time.Second,
		WriteTimeout:        10 * time.Second,
		MaxIdleConnDuration: 90 * time.Second,

		ReadBufferSize:  4 * 1024,
		WriteBufferSize: 4 * 1024,

		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
	}
}

// ClientOption mutates ClientOptions.
type ClientOption func(*ClientOptions)

// WithDialTimeout sets the connection establishment timeout.
func WithDialTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.DialTimeout = timeout
	}
}

// WithReadTimeout sets the per-read deadline for transports that support it.
func WithReadTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.ReadTimeout = timeout
	}
}

// WithWriteTimeout sets the per-write deadline for transports that support it.
func WithWriteTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.WriteTimeout = timeout
	}
}

// WithKeepAlive configures keepalive behavior for transports that support it.
func WithKeepAlive(keepAlive bool, period time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.KeepAlive = keepAlive
		opts.KeepAlivePeriod = period
	}
}

// WithRetry records transport-level retry settings in ClientOptions.
func WithRetry(maxRetries int, interval time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.MaxRetries = maxRetries
		opts.RetryInterval = interval
	}
}

// WithBufferSize sets socket read and write buffer sizes for transports that
// support them.
func WithBufferSize(readSize, writeSize int) ClientOption {
	return func(opts *ClientOptions) {
		opts.ReadBufferSize = readSize
		opts.WriteBufferSize = writeSize
	}
}

// ServerOptions groups transport-level server tuning knobs used by concrete
// server implementations such as TCP.
type ServerOptions struct {
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	HandlerTimeout        time.Duration // max time a handler goroutine may run; 0 = no limit
	MaxConcurrentRequests int
	WorkerPoolSize        int
	ReadBufferSize        int
	WriteBufferSize       int
	MaxRequestBodySize    int64
	MaxConnections        int
	Logger                logger.Logger // never nil; defaults to logger.Nop()
}

// DefaultServerOptions returns the default transport server settings.
func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		HandlerTimeout:        0, // unlimited by default
		MaxConcurrentRequests: 0,
		WorkerPoolSize:        8,
		ReadBufferSize:        4 * 1024,
		WriteBufferSize:       4 * 1024,
		MaxRequestBodySize:    10 * 1024 * 1024,
		MaxConnections:        10000,
		Logger:                logger.Nop(),
	}
}

// ServerOption mutates ServerOptions.
type ServerOption func(*ServerOptions)

// WithServerTimeout sets transport-level read and write deadlines.
func WithServerTimeout(read, write time.Duration) ServerOption {
	return func(opts *ServerOptions) {
		opts.ReadTimeout = read
		opts.WriteTimeout = write
	}
}

// WithHandlerTimeout limits how long a request handler may run; zero disables
// the limit.
func WithHandlerTimeout(timeout time.Duration) ServerOption {
	return func(opts *ServerOptions) {
		opts.HandlerTimeout = timeout
	}
}

// WithMaxConcurrentRequests limits the number of handlers that may run at the
// same time; zero disables the limit.
func WithMaxConcurrentRequests(maxConcurrentRequests int) ServerOption {
	return func(opts *ServerOptions) {
		opts.MaxConcurrentRequests = maxConcurrentRequests
	}
}

// WithWorkerPool sets the worker-pool size used by the transport.
func WithWorkerPool(size int) ServerOption {
	return func(opts *ServerOptions) {
		opts.WorkerPoolSize = size
	}
}

// WithServerBufferSize sets socket read and write buffer sizes for transports
// that support them.
func WithServerBufferSize(readSize, writeSize int) ServerOption {
	return func(opts *ServerOptions) {
		opts.ReadBufferSize = readSize
		opts.WriteBufferSize = writeSize
	}
}

// WithMaxRequestBodySize rejects requests whose encoded body exceeds size.
func WithMaxRequestBodySize(size int64) ServerOption {
	return func(opts *ServerOptions) {
		opts.MaxRequestBodySize = size
	}
}

// WithLogger installs the logger the transport server uses for events that
// have no caller to return an error to, such as a connection handler that
// fails after the accept loop has moved on. Passing nil is a no-op, so the
// server never has to nil-check.
func WithLogger(l logger.Logger) ServerOption {
	return func(o *ServerOptions) {
		if l != nil {
			o.Logger = l
		}
	}
}
