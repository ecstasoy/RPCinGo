// Kunhua Huang, 12/29/2025

package transport

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

type TCPServerTransport struct {
	addr     string
	listener net.Listener
	handler  RequestHandler
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
}

var _ ServerTransport = (*TCPServerTransport)(nil)

func NewTCPServerTransport() *TCPServerTransport {
	return &TCPServerTransport{}
}

func (s *TCPServerTransport) Listen(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return fmt.Errorf("already listening on %s", s.addr)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	s.addr = address
	s.listener = listener

	fmt.Println("server being listen on", s.addr)
	return nil
}

func (s *TCPServerTransport) Serve(handler RequestHandler) error {
	s.mu.Lock()
	if s.listener == nil {
		s.mu.Unlock()
		return fmt.Errorf("server not listening, call Listen() first")
	}
	s.handler = handler
	s.mu.Unlock()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()

			if closed {
				return nil
			}

			fmt.Println("accept connection failed:", err)
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *TCPServerTransport) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("close connection failed:", err)
		}
	}(conn)

	fmt.Println("accepted connection from", conn.RemoteAddr().String())

	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			if err != io.EOF {
				fmt.Println("read length failed:", err)
			}
			return
		}

		length := binary.BigEndian.Uint32(lengthBuf)

		if length > 10*1024*1024 {
			fmt.Println("message too large:", length)
			return
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			fmt.Println("read data failed:", err)
			return
		}

		fmt.Println("received data from", conn.RemoteAddr().String(), "length:", length)

		response := s.handler(data)

		respLengthBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(respLengthBuf, uint32(len(response)))
		if _, err := conn.Write(respLengthBuf); err != nil {
			fmt.Println("write response length failed:", err)
			return
		}

		if _, err := conn.Write(response); err != nil {
			fmt.Println("write response data failed:", err)
			return
		}

		fmt.Println("sent response to", conn.RemoteAddr().String(), "length:", len(response))
	}
}

func (s *TCPServerTransport) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.listener != nil {
		err := s.listener.Close()
		if err != nil {
			fmt.Println("close listener failed:", err)
			return err
		}
	}

	s.wg.Wait()

	fmt.Println("server on", s.addr, "closed")
	return nil
}
