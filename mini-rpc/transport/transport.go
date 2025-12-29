// Kunhua Huang, 12/29/2025

package transport

import "io"

type ClientTransport interface {
	Connect() error
	Send(data []byte) ([]byte, error)
	Close() error
}

type ServerTransport interface {
	Listen(address string) error
	Serve(handler RequestHandler) error
	Close() error
}

type RequestHandler func(data []byte) []byte

type Connection interface {
	io.ReadWriter
	io.Closer

	RemoteAddr() string
}
