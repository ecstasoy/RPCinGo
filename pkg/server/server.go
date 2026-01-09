// Kunhua Huang 2026

package server

import (
	"RPCinGo/pkg/interceptor"
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"RPCinGo/pkg/codec"
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/registry"
	"RPCinGo/pkg/transport"
	"RPCinGo/pkg/transport/tcp"
)

type Server struct {
	opts      *serverOptions
	registry  *ServiceRegistry
	Transport *tcp.Server
	codec     codec.Codec

	// Registry integration
	serviceInstance *registry.ServiceInstance
	stopHeartbeat   chan struct{}

	interceptors []interceptor.Interceptor
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
		Transport: tcp.NewServer(
			options.codecType,
			options.compressType,
			transport.WithServerTimeout(options.readTimeout, options.writeTimeout),
			transport.WithWorkerPool(options.workerPoolSize),
			transport.WithMaxConcurrentRequests(options.maxConcurrent),
		),
		codec:         codec.Get(options.codecType),
		stopHeartbeat: make(chan struct{}),
	}
}

func (s *Server) RegisterMethod(service, method string, handler MethodHandler) error {
	return s.registry.RegisterMethod(service, method, handler)
}

func (s *Server) RegisterService(serviceName string, serviceImpl interface{}) error {
	return s.registry.RegisterService(serviceName, serviceImpl)
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.Transport.Listen(ctx, s.opts.address); err != nil {
		return fmt.Errorf("failed to listen tcp transport: %w", err)
	}

	if s.opts.enableRegistry && s.opts.registry != nil {
		if err := s.registerService(); err != nil {
			return fmt.Errorf("register service: %v", err)
		}

		go func() {
			err := s.startHeartbeat()
			if err != nil {
				fmt.Printf("heartbeat error: %v\n", err)
			}
		}()
	}

	handler := func(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
		return s.HandleRequest(ctx, req)
	}

	return s.Transport.Serve(ctx, handler)
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

	result, err := chain.Intercept(ctx, req, invoker)

	if err != nil {
		code, msg := mapError(err, req.Service, req.Method)
		return protocol.NewErrorResponse(req.ID, protocol.NewError(code, msg)), nil
	}

	return protocol.NewSuccessResponse(req.ID, result), nil
}

func (s *Server) Addr() string {
	if s.Transport.Addr() != nil {
		return s.Transport.Addr().String()
	}
	return ""
}

func (s *Server) registerService() error {
	addr := s.Transport.Addr().String()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	s.serviceInstance = registry.NewServiceInstance(
		s.opts.serviceName,
		host,
		port,
	)

	if s.opts.serviceVersion != "" {
		s.serviceInstance.Version = s.opts.serviceVersion
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.opts.registry.Register(ctx, s.serviceInstance); err != nil {
		return fmt.Errorf("register to registry: %w", err)
	}

	return nil
}

func (s *Server) startHeartbeat() error {
	ticker := time.NewTicker(s.opts.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopHeartbeat:
			return nil
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.opts.registry.Heartbeat(ctx, s.opts.serviceName, s.serviceInstance.ID)
			cancel()
			if err != nil {
				return fmt.Errorf("heartbeat failed: %w", err)
			}
		}
	}
}

func (s *Server) Stop() error {
	close(s.stopHeartbeat)

	if s.opts.enableRegistry && s.opts.registry != nil && s.serviceInstance != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := s.opts.registry.Deregister(ctx, s.opts.serviceName, s.serviceInstance.ID)
		if err != nil {
			return fmt.Errorf("deregister from registry: %w", err)
		}
	}

	return s.Transport.Close()
}
