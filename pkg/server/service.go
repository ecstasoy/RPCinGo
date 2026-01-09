// Kunhua Huang 2026

package server

import (
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/registry"
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type MethodHandler func(ctx context.Context, req *protocol.Request) (interface{}, error)

type Service struct {
	name    string
	methods map[string]MethodHandler
	mu      sync.RWMutex
}

func NewService(name string) *Service {
	return &Service{
		name:    name,
		methods: make(map[string]MethodHandler),
	}
}

func (s *Service) RegisterMethod(method string, handler MethodHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.methods[method]; exists {
		return fmt.Errorf("method %s already registered in service %s", method, s.name)
	}

	s.methods[method] = handler
	return nil
}

func (s *Service) GetMethod(method string) (MethodHandler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	handler, ok := s.methods[method]
	return handler, ok
}

func (s *Service) Methods() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	methods := make([]string, 0, len(s.methods))
	for m := range s.methods {
		methods = append(methods, m)
	}
	return methods
}

type ServiceRegistry struct {
	services map[string]*Service
	mu       sync.RWMutex
}

func newServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]*Service),
	}
}

func (sr *ServiceRegistry) RegisterMethod(service, method string, handler MethodHandler) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	svc, exists := sr.services[service]
	if !exists {
		svc = NewService(service)
		sr.services[service] = svc
	}

	return svc.RegisterMethod(method, handler)
}

func (sr *ServiceRegistry) GetHandler(service, method string) (MethodHandler, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	svc, exists := sr.services[service]
	if !exists {
		return nil, fmt.Errorf("service %s not found", service)
	}

	handler, ok := svc.GetMethod(method)
	if !ok {
		return nil, fmt.Errorf("method %s not found in service %s", method, service)
	}

	return handler, nil
}

func (sr *ServiceRegistry) Services() []string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	services := make([]string, 0, len(sr.services))
	for s := range sr.services {
		services = append(services, s)
	}
	return services
}
