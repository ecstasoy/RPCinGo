// Kunhua Huang 2025

package transport

import (
	"RPCinGo/pkg/protocol"
	"context"
	"io"
	"net"
	"time"
)

// ClientTransport Thank God Golang has context to manage timeouts and cancellations
type ClientTransport interface {
	Dial(ctx context.Context, addr string) error
	Send(ctx context.Context, data []byte) ([]byte, error)
	Close() error
	IsConnected() bool
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type ServerTransport interface {
	Listen(ctx context.Context, addr string) error
	Serve(ctx context.Context, handler Handler) error
	Close() error
	Addr() net.Addr
}

type Handler func(ctx context.Context, req *protocol.Request) (*protocol.Response, error)

// Connection embeds io.ReadWriter and io.Closer to use std interfaces for network connections
type Connection interface {
	io.ReadWriter
	io.Closer

	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
