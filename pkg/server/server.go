// Kunhua Huang 2026

package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/codec"
	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/logger"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry"
	"github.com/ecstasoy/RPCinGo/pkg/transport"
	"github.com/ecstasoy/RPCinGo/pkg/transport/tcp"
)

// Server is the high-level RPC server that combines service registration,
// interceptor execution, TCP transport, and optional service registry
// integration.
type Server struct {
	opts     *serverOptions
	registry *ServiceRegistry
	// Transport exposes the underlying TCP transport used to accept
	// connections and serve requests.
	Transport *tcp.Server
	codec     codec.Codec
	log       logger.Logger

	// Registry integration
	serviceInstance  *registry.ServiceInstance
	stopHeartbeat    chan struct{}
	stopHeartbeatOne sync.Once

	interceptors []interceptor.Interceptor
}

// Invoker is the terminal request handler executed after server interceptors
// run.
type Invoker func(ctx context.Context, req *protocol.Request) (any, error)

// NewServer builds a Server with the supplied options and initializes the
// underlying transport, codec, logger, and interceptor chain.
func NewServer(opts ...Option) *Server {
	options := defaultServerOptions()
	for _, o := range opts {
		o(options)
	}

	log := options.logger
	if log == nil {
		log = logger.New()
	}

	// Translate the named server options into transport options, then append
	// any raw transport options the caller supplied via WithTransportOptions.
	// The raw options come last so they take precedence and so every transport
	// knob is reachable through a single server constructor.
	transportOpts := []transport.ServerOption{
		transport.WithServerTimeout(options.readTimeout, options.writeTimeout),
		transport.WithHandlerTimeout(options.handlerTimeout),
		transport.WithWorkerPool(options.workerPoolSize),
		transport.WithMaxConcurrentRequests(options.maxConcurrent),
	}
	transportOpts = append(transportOpts, options.transportOptions...)

	return &Server{
		opts:          options,
		registry:      newServiceRegistry(),
		Transport:     tcp.NewServer(options.codecType, options.compressType, transportOpts...),
		codec:         codec.Get(options.codecType),
		log:           log,
		stopHeartbeat: make(chan struct{}),
		interceptors:  options.interceptors,
	}
}

// RegisterMethod registers a single MethodHandler under service and method.
func (s *Server) RegisterMethod(service, method string, handler MethodHandler) error {
	return s.registry.RegisterMethod(service, method, handler)
}

// RegisterService reflects over serviceImpl and registers all eligible exported
// methods under serviceName.
func (s *Server) RegisterService(serviceName string, serviceImpl interface{}) error {
	return s.registry.RegisterService(serviceName, serviceImpl)
}

// Start begins listening and serving requests until ctx is canceled or the
// transport stops. If registry integration is enabled, Start also registers the
// service instance and starts heartbeats.
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
//
// Usage:
//
//	srv.Use(
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

// HandleRequest executes the interceptor chain, dispatches the request to the
// registered method, and converts handler errors into protocol error
// responses.
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

// Addr returns the bound listener address or an empty string before the
// transport starts listening.
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

// Stop stops registry heartbeats, deregisters the service instance when
// enabled, and closes the underlying transport.
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
