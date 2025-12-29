// Kunhua Huang, 12/29/2025

package server

import (
	"fmt"
	"mini-rpc/codec"
	"mini-rpc/protocol"
	"mini-rpc/transport"
	"sync"
)

type ServiceFunc func(method string, args []interface{}) (interface{}, error)

type Server struct {
	addr      string
	transport transport.ServerTransport
	codec     codec.Codec
	services  map[string]ServiceFunc
	mu        sync.RWMutex
}

func NewServer(addr string) *Server {
	return &Server{
		addr:      addr,
		transport: transport.NewTCPServerTransport(),
		codec:     codec.GetCodec(codec.JSONCodecType),
		services:  make(map[string]ServiceFunc),
	}
}

func (s *Server) Register(ServiceName string, handler ServiceFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.services[ServiceName]; exists {
		panic("service already registered: " + ServiceName)
	}

	s.services[ServiceName] = handler
	fmt.Printf("registered service: %s\n", ServiceName)
}

func (s *Server) Start() error {
	if err := s.transport.Listen(s.addr); err != nil {
		return fmt.Errorf("listen failed: %w\n", err)
	}

	fmt.Printf("[Server] Starting server on %s\n", s.addr)

	return s.transport.Serve(s.handleRequest)
}

func (s *Server) handleRequest(data []byte) []byte {
	var req protocol.Request
	if err := s.codec.Decode(data, &req); err != nil {
		resp := protocol.NewResponse(0, nil, fmt.Errorf("decode request failed: %w", err))
		respData, _ := s.codec.Encode(resp)
		return respData
	}

	fmt.Printf("[Server] Received request: %+v\n", req)

	s.mu.RLock()
	handler, exists := s.services[req.Service]
	s.mu.RUnlock()

	if !exists {
		resp := protocol.NewResponse(req.ID, nil, fmt.Errorf("service %s not found", req.Service))
		resData, _ := s.codec.Encode(resp)
		return resData
	}

	result, err := handler(req.Method, req.Args)

	fmt.Printf("[Server] Handler result: %v, error: %v\n", result, err)

	resp := protocol.NewResponse(req.ID, result, err)

	respData, err := s.codec.Encode(resp)
	if err != nil {
		resp := protocol.NewResponse(req.ID, nil, fmt.Errorf("encode response failed: %w", err))
		respData, _ := s.codec.Encode(resp)
		return respData
	}

	fmt.Printf("[Server] Sending response data: %v\n", respData)
	return respData
}

func (s *Server) Stop() error {
	fmt.Println("[Server] Stopping server...")
	return s.transport.Close()
}
