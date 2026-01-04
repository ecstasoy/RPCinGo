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

func (s *Server) handleRequest(ctx context.Context, data []byte) ([]byte, error) {
	var req protocol.Request
	if err := s.codec.Decode(data, &req); err != nil {
		return s.encodeErrorResponse(0, protocol.ErrorCodeInvalidArgument,
			"decode request failed")
	}

	handler, err := s.registry.GetHandler(req.Service, req.Method)
	if err != nil {
		return s.encodeErrorResponse(req.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("method %s.%s not found", req.Service, req.Method))
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
