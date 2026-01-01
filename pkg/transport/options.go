package transport

import "time"

// ------------------- Client Options -------------------

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

type ClientOption func(*ClientOptions)

func WithDialTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.DialTimeout = timeout
	}
}

func WithReadTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.ReadTimeout = timeout
	}
}

func WithWriteTimeout(timeout time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.WriteTimeout = timeout
	}
}

func WithKeepAlive(keepAlive bool, period time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.KeepAlive = keepAlive
		opts.KeepAlivePeriod = period
	}
}

func WithRetry(maxRetries int, interval time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		opts.MaxRetries = maxRetries
		opts.RetryInterval = interval
	}
}

func WithBufferSize(readSize, writeSize int) ClientOption {
	return func(opts *ClientOptions) {
		opts.ReadBufferSize = readSize
		opts.WriteBufferSize = writeSize
	}
}

// ------------------- Server Options -------------------

type ServerOptions struct {
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	MaxConcurrentRequests int
	WorkerPoolSize        int
	ReadBufferSize        int
	WriteBufferSize       int
	MaxRequestBodySize    int64
	MaxConnections        int
}

func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		MaxConcurrentRequests: 0,
		WorkerPoolSize:        8,
		ReadBufferSize:        4 * 1024,
		WriteBufferSize:       4 * 1024,
		MaxRequestBodySize:    10 * 1024 * 1024,
		MaxConnections:        10000,
	}
}

type ServerOption func(*ServerOptions)

func WithServerTimeout(read, write time.Duration) ServerOption {
	return func(opts *ServerOptions) {
		opts.ReadTimeout = read
		opts.WriteTimeout = write
	}
}

func WithMaxConcurrentRequests(maxConcurrentRequests int) ServerOption {
	return func(opts *ServerOptions) {
		opts.MaxConcurrentRequests = maxConcurrentRequests
	}
}

func WithWorkerPool(size int) ServerOption {
	return func(opts *ServerOptions) {
		opts.WorkerPoolSize = size
	}
}

func WithServerBufferSize(readSize, writeSize int) ServerOption {
	return func(opts *ServerOptions) {
		opts.ReadBufferSize = readSize
		opts.WriteBufferSize = writeSize
	}
}

func WithMaxRequestBodySize(size int64) ServerOption {
	return func(opts *ServerOptions) {
		opts.MaxRequestBodySize = size
	}
}
