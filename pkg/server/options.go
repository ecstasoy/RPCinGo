// Kunhua Huang 2026

package server

import (
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

type serverOptions struct {
	address        string
	codecType      protocol.CodecType
	compressType   protocol.CompressType
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxConcurrent  int
	workerPoolSize int
}

func defaultServerOptions() *serverOptions {
	return &serverOptions{
		address:        ":8080",
		codecType:      protocol.CodecTypeJSON,
		compressType:   protocol.CompressTypeNone,
		readTimeout:    10 * time.Second,
		writeTimeout:   10 * time.Second,
		maxConcurrent:  0,
		workerPoolSize: 8,
	}
}

type Option func(*serverOptions)

func WithAddress(addr string) Option {
	return func(o *serverOptions) {
		o.address = addr
	}
}

func WithCodec(codec protocol.CodecType, compress protocol.CompressType) Option {
	return func(o *serverOptions) {
		o.codecType = codec
		o.compressType = compress
	}
}

func WithTimeout(read, write time.Duration) Option {
	return func(o *serverOptions) {
		o.readTimeout = read
		o.writeTimeout = write
	}
}

func WithConcurrency(maxConcurrent, workerPoolSize int) Option {
	return func(o *serverOptions) {
		o.maxConcurrent = maxConcurrent
		o.workerPoolSize = workerPoolSize
	}
}
