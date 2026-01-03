package client

import (
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

type clientOptions struct {
	codecType      protocol.CodecType
	compressType   protocol.CompressType
	maxConnections int
	minConnections int
	idleTimeout    time.Duration
	callTimeout    time.Duration
}

func defaultOptions() *clientOptions {
	return &clientOptions{
		codecType:      protocol.CodecTypeJSON,
		compressType:   protocol.CompressTypeNone,
		maxConnections: 100,
		minConnections: 10,
		idleTimeout:    90 * time.Second,
		callTimeout:    5 * time.Second,
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
