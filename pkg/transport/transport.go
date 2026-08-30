// Kunhua Huang 2025

package transport

import (
	"context"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"io"
	"net"
	"time"
)

// ClientTransport is the transport abstraction used by RPC clients.
type ClientTransport interface {
	Dial(ctx context.Context, addr string) error
	SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
	Close() error
	IsConnected() bool
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// ServerTransport is the transport abstraction used by RPC servers.
type ServerTransport interface {
	Listen(ctx context.Context, addr string) error
	Serve(ctx context.Context, handler Handler) error
	Close() error
	Addr() net.Addr
}

// Handler processes one decoded request and returns its response.
type Handler func(ctx context.Context, req *protocol.Request) (*protocol.Response, error)

// Connection embeds io.ReadWriter and io.Closer to use std interfaces for network connections
// while exposing standard network deadline and address methods.
type Connection interface {
	io.ReadWriter
	io.Closer

	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
