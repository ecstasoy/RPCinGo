// Kunhua Huang 2026

package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/transport"
)

type Server struct {
	address  string
	opts     *transport.ServerOptions
	codec    *ProtocolCodec
	handler  transport.Handler
	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.RWMutex
	serving  bool
	closed   bool
	// atomic counters
	activeConnections int64
	totalConnections  int64
	// semaphore to limit max concurrent connections
	connSemaphore chan struct{}
	// semaphore to limit max concurrent requests
	reqSemaphore chan struct{}
}

func NewServer(
	codecType protocol.CodecType,
	compressType protocol.CompressType,
	options ...transport.ServerOption,
) *Server {
	opts := transport.DefaultServerOptions()

	for _, o := range options {
		o(opts)
	}

	server := &Server{
		opts:  opts,
		codec: NewProtocolCodec(codecType, compressType),
	}

	if opts.MaxConnections > 0 {
		server.connSemaphore = make(chan struct{}, opts.MaxConnections)
	}

	if opts.MaxConcurrentRequests > 0 {
		server.reqSemaphore = make(chan struct{}, opts.MaxConcurrentRequests)
	}

	return server
}

func (s *Server) Listen(ctx context.Context, addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return fmt.Errorf("already listening on %s", addr)
	}

	lc := net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	s.address = listener.Addr().String()

	return nil
}

func (s *Server) Serve(ctx context.Context, handler transport.Handler) error {
	s.mu.Lock()

	if s.listener == nil {
		s.mu.Unlock()
		return fmt.Errorf("not listening on %s", s.address)
	}

	if s.serving {
		s.mu.Unlock()
		return fmt.Errorf("already serving on %s", s.address)
	}

	s.serving = true
	s.handler = handler
	s.mu.Unlock()

	// start a goroutine to accept connections
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		s.listener.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-done:
				return ctx.Err()
			default:
			}

			s.mu.RLock()
			if s.closed {
				s.mu.RUnlock()
				return nil
			}

			s.mu.RUnlock()

			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			return fmt.Errorf("accept connection failed: %w", err)
		}

		if s.connSemaphore != nil {
			select {
			case s.connSemaphore <- struct{}{}:
				// acquired semaphore
			default:
				// max connections reached
				err := conn.Close()
				if err != nil {
					return fmt.Errorf("close connection failed: %w", err)
				}
				continue
			}
		}

		atomic.AddInt64(&s.activeConnections, 1)
		atomic.AddInt64(&s.totalConnections, 1)

		s.wg.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) error {
	defer s.CloseConnection(conn)

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		err := tcpConn.SetKeepAlive(true)
		if err != nil {
			return err
		}
		err = tcpConn.SetKeepAlivePeriod(30 * time.Second)
		if err != nil {
			return err
		}
		err = tcpConn.SetNoDelay(true)
		if err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if s.opts.ReadTimeout > 0 {
			err := conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))
			if err != nil {
				return fmt.Errorf("set read deadline failed: %w", err)
			}
		}

		header, req, err := s.codec.ReadRequest(conn)
		if err != nil {
			return fmt.Errorf("read request failed: %w", err)
		}

		reqCtx, cancel := context.WithTimeout(ctx, s.opts.WriteTimeout)

		if s.reqSemaphore != nil {
			select {
			case s.reqSemaphore <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			default:
				resp := protocol.NewErrorResponse(header.RequestID, protocol.NewError(protocol.ErrorCodeUnavailable, "too many concurrent requests"))
				if err := s.codec.WriteResponse(conn, resp); err != nil {
					return fmt.Errorf("write error response failed: %w", err)
				}
				continue
			}
		}

		respData, err := s.handler(reqCtx, reqData)
		cancel()

		if s.reqSemaphore != nil {
			<-s.reqSemaphore
		}

		if handlerErr != nil {
			resp = protocol.NewErrorResponse(header.RequestID, protocol.NewError(protocol.ErrorCodeInternal, handlerErr.Error()))
		}

		if s.opts.WriteTimeout > 0 {
			err := conn.SetWriteDeadline(time.Now().Add(s.opts.WriteTimeout))
			if err != nil {
				return fmt.Errorf("set write deadline failed: %w", err)
			}
		}

		if err := s.codec.WriteResponse(conn, resp); err != nil {
			return fmt.Errorf("write response failed: %w", err)
		}

		err = conn.SetReadDeadline(time.Time{})
		if err != nil {
			return fmt.Errorf("set read deadline failed: %w", err)
		}
		err = conn.SetWriteDeadline(time.Time{})
		if err != nil {
			return fmt.Errorf("set write deadline failed: %w", err)
		}
	}
}

func (s *Server) CloseConnection(conn net.Conn) error {
	err := conn.Close()
	if err != nil {
		return fmt.Errorf("close connection failed: %w", err)
	}

	atomic.AddInt64(&s.activeConnections, -1)

	if s.connSemaphore != nil {
		<-s.connSemaphore
	}

	s.wg.Done()

	return nil
}

func (s *Server) sendErrorResponse(conn net.Conn, requestID uint64, error *protocol.Error) error {
	resp := protocol.NewErrorResponse(requestID, error)
	err := s.codec.WriteResponse(conn, resp)
	if err != nil {
		return fmt.Errorf("send error response failed: %w", err)
	}

	return nil
}

func (s *Server) Close() error {
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
			return fmt.Errorf("close listener failed: %w", err)
		}
	}

	s.wg.Wait()

	return nil
}

func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *Server) Stats() ServerStats {
	return ServerStats{
		ActiveConnections: atomic.LoadInt64(&s.activeConnections),
		TotalConnections:  atomic.LoadInt64(&s.totalConnections),
		Address:           s.address,
	}
}

type ServerStats struct {
	ActiveConnections int64
	TotalConnections  int64
	Address           string
}
