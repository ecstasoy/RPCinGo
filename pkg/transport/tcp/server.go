// Kunhua Huang 2026

package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/transport"
)

// Server implements the TCP server transport for framed RPC requests.
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

// NewServer constructs a TCP transport server using codecType and compressType
// for request and response bodies.
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

// Listen binds the server to addr without starting the accept loop.
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

// Serve accepts connections on the bound listener and dispatches decoded
// requests to handler until ctx is canceled or the server is closed.
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
		_ = s.listener.Close()
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
		go func(conn net.Conn) {
			if err := s.handleConnection(ctx, conn); err != nil {
				s.opts.Logger.Error("connection handler failed",
					"remote", conn.RemoteAddr().String(),
					"err", err)
			}
		}(conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) error {
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()
	defer func() { _ = s.CloseConnection(conn) }()

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

		if s.opts.ReadBufferSize > 0 {
			if err := tcpConn.SetReadBuffer(s.opts.ReadBufferSize); err != nil {
				return fmt.Errorf("set read buffer size failed: %w", err)
			}
		}

		if s.opts.WriteBufferSize > 0 {
			if err := tcpConn.SetWriteBuffer(s.opts.WriteBufferSize); err != nil {
				return fmt.Errorf("set write buffer size failed: %w", err)
			}
		}
	}

	writeCh := make(chan *protocol.Response, 64)

	// writer goroutine: 串行写响应，防止帧交错
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		for resp := range writeCh {
			if s.opts.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(s.opts.WriteTimeout))
			}

			if err := s.codec.WriteResponse(conn, resp); err != nil {
				_ = conn.Close()
				connCancel()
				return
			}
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}()

	// handlersWg 追踪正在运行的 handler goroutine
	// 必须等它们全部结束后才能 close(writeCh)，否则会 panic
	var handlersWg sync.WaitGroup

outer:
	for {
		select {
		case <-ctx.Done():
			break outer
		default:
		}

		if s.opts.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))
		}

		header, req, err := s.codec.ReadRequest(conn)
		if err != nil {
			break outer
		}
		_ = conn.SetReadDeadline(time.Time{})

		if s.opts.MaxRequestBodySize > 0 && int64(header.BodyLength) > s.opts.MaxRequestBodySize {
			select {
			case writeCh <- protocol.NewErrorResponse(header.RequestID,
				protocol.NewError(protocol.ErrorRequestEntityTooLarge, "request body too large")):
			case <-connCtx.Done():
				break outer
			}
		}

		if s.reqSemaphore != nil {
			select {
			case s.reqSemaphore <- struct{}{}:
			default:
				select {
				case writeCh <- protocol.NewErrorResponse(header.RequestID,
					protocol.NewError(protocol.ErrorCodeUnavailable, "too many concurrent requests")):
				case <-connCtx.Done():
					break outer
				}
			}
		}

		handlersWg.Add(1)
		go func(header *protocol.Header, req *protocol.Request) {
			defer handlersWg.Done()
			defer func() {
				if s.reqSemaphore != nil {
					<-s.reqSemaphore
				}
			}()

			var reqCtx context.Context
			var cancel context.CancelFunc
			if s.opts.HandlerTimeout > 0 {
				reqCtx, cancel = context.WithTimeout(connCtx, s.opts.HandlerTimeout)
			} else {
				reqCtx, cancel = context.WithCancel(connCtx)
			}
			defer cancel()

			resp, handlerErr := s.handler(reqCtx, req)
			if handlerErr != nil {
				resp = protocol.NewErrorResponse(header.RequestID,
					protocol.NewError(protocol.ErrorCodeInternal, handlerErr.Error()))
			}
			select {
			case writeCh <- resp:
			case <-connCtx.Done():
				return
			}
		}(header, req)
	}

	// 等所有 handler 把响应写入 writeCh 后再关闭，避免 send on closed channel
	handlersWg.Wait()
	close(writeCh)
	writerWg.Wait()
	return nil
}

// CloseConnection closes conn and updates the server's connection accounting.
func (s *Server) CloseConnection(conn net.Conn) error {
	err := conn.Close()

	atomic.AddInt64(&s.activeConnections, -1)
	if s.connSemaphore != nil {
		<-s.connSemaphore
	}
	s.wg.Done()

	if err != nil {
		return fmt.Errorf("close connection failed: %w", err)
	}

	return nil
}

// Close stops accepting new connections and waits for active handlers to exit.
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

// Addr returns the bound listener address, or nil before Listen succeeds.
func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

// Stats returns point-in-time connection counters for the server.
func (s *Server) Stats() ServerStats {
	return ServerStats{
		ActiveConnections: atomic.LoadInt64(&s.activeConnections),
		TotalConnections:  atomic.LoadInt64(&s.totalConnections),
		Address:           s.address,
	}
}

// Options returns a copy of the transport options the server was built with.
// It lets higher layers verify which transport knobs are in effect (for
// example, that a server-level HandlerTimeout reached the transport).
func (s *Server) Options() transport.ServerOptions {
	return *s.opts
}

// ServerStats summarizes TCP server connection counters.
type ServerStats struct {
	ActiveConnections int64
	TotalConnections  int64
	Address           string
}
