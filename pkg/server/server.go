// Kunhua Huang 2026

package server

import (
	"context"
	"fmt"

	"github.com/ecstasoy/RPCinGo/pkg/codec"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/transport"
	"github.com/ecstasoy/RPCinGo/pkg/transport/tcp"
)

type Server struct {
	opts      *serverOptions
	registry  *ServiceRegistry
	transport *tcp.Server
	codec     codec.Codec
}

type Invoker func(ctx context.Context, req *protocol.Request) (any, error)

func NewServer(opts ...Option) *Server {
	options := defaultServerOptions()
	for _, o := range opts {
		o(options)
	}

	return &Server{
		opts:     options,
		registry: newServiceRegistry(),
		transport: tcp.NewServer(
			options.codecType,
			options.compressType,
			transport.WithServerTimeout(options.readTimeout, options.writeTimeout),
			transport.WithWorkerPool(options.workerPoolSize),
			transport.WithMaxConcurrentRequests(options.maxConcurrent),
		),
		codec: codec.Get(options.codecType),
	}
}

func (s *Server) RegisterMethod(service, method string, handler MethodHandler) error {
	return s.registry.RegisterMethod(service, method, handler)
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.transport.Listen(ctx, s.opts.address); err != nil {
		return fmt.Errorf("failed to listen tcp transport: %w", err)
	}

	handler := func(ctx context.Context, reqData []byte) ([]byte, error) {
		return s.handleRequest(ctx, reqData)
	}

	return s.transport.Serve(ctx, handler)
}

// Use adds interceptors to the server's interceptor chain.
// usage:
// srv.Use(
//
//		interceptor.Recovery(),
//		interceptor.Logging(nil),
//		interceptor.Metrics(),
//	)
//
// The interceptors will be executed in the order they are added.
func (s *Server) Use(interceptors ...interceptor.Interceptor) {
	s.interceptors = append(s.interceptors, interceptors...)
}

func (s *Server) HandleRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	chain := interceptor.NewChain(s.interceptors...)

	invoker := func(ctx context.Context, request *protocol.Request) (any, error) {
		handler, err := s.registry.GetHandler(request.Service, request.Method)
		if err != nil {
			return nil, fmt.Errorf("get handler: %w", err)
		}
		return handler(ctx, request)
	}

	result, err := handler(req.Args)
	if err != nil {
		return s.encodeErrorResponse(req.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("method %s.%s execution failed: %v", req.Service, req.Method, err))
	}

	resp := protocol.NewSuccessResponse(req.ID, result)
	return s.codec.Encode(resp)
}

func (s *Server) encodeErrorResponse(reqID uint64, code int32, msg string) ([]byte, error) {
	err := protocol.NewError(code, msg)
	resp := protocol.NewErrorResponse(reqID, err)
	return s.codec.Encode(resp)
}

func (s *Server) Stop() error {
	return s.transport.Close()
}

func (s *Server) Addr() string {
	if s.transport.Addr() != nil {
		return s.transport.Addr().String()
	}
	return ""
}
