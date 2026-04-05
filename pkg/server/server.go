// Kunhua Huang 2026

package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"RPCinGo/pkg/codec"
	"RPCinGo/pkg/interceptor"
	"RPCinGo/pkg/logger"
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
	log       logger.Logger

	// Registry integration
	serviceInstance  *registry.ServiceInstance
	stopHeartbeat    chan struct{}
	stopHeartbeatOne sync.Once

	interceptors []interceptor.Interceptor
}

type Invoker func(ctx context.Context, req *protocol.Request) (any, error)

func NewServer(opts ...Option) *Server {
	options := defaultServerOptions()
	for _, o := range opts {
		o(options)
	}

	log := options.logger
	if log == nil {
		log = logger.New()
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
		log:           log,
		stopHeartbeat: make(chan struct{}),
		interceptors:  options.interceptors,
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
			if err := s.startHeartbeat(); err != nil {
				s.log.Error("heartbeat stopped", "error", err)
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

	resp := protocol.NewSuccessResponse(req.ID, result)
	if spanID, ok := req.GetMetadata(protocol.MetaKeySpanID); ok && spanID != "" {
		resp.SetMetadata(protocol.MetaKeySpanID, spanID)
	}
	return resp, nil
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
	s.stopHeartbeatOne.Do(func() { close(s.stopHeartbeat) })

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
