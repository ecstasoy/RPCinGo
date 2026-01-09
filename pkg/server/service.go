// Kunhua Huang 2026

package server

import (
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/registry"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/protobuf/proto"
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
		return nil, fmt.Errorf("%w: service=%s", registry.ErrNotFound, service)
	}

	handler, ok := svc.GetMethod(method)
	if !ok {
		return nil, fmt.Errorf("%w: service=%s method=%s", registry.ErrNotFound, service, method)
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

func (sr *ServiceRegistry) RegisterService(serviceName string, serviceImpl any) error {
	if serviceImpl == nil {
		return fmt.Errorf("service implementation is nil")
	}

	recv := reflect.ValueOf(serviceImpl)
	typ := recv.Type()

	if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("service must be a pointer to struct, got %s", typ.String())
	}

	registered := 0

	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.PkgPath != "" {
			continue
		}

		kind, ok := methodKind(m.Type)
		if !ok {
			continue
		}

		handler := makeMethodHandler(recv, m, kind)

		if err := sr.RegisterMethod(serviceName, m.Name, handler); err != nil {
			return fmt.Errorf("register method %s.%s: %w", serviceName, m.Name, err)
		}
		registered++
	}

	if registered == 0 {
		return fmt.Errorf("no valid rpc methods found in service %q (%s)", serviceName, typ.String())
	}
	return nil
}

type rpcMethodKind int

const (
	rpcMethodArgsOnly rpcMethodKind = iota
	rpcMethodCtxArgs
	rpcMethodTyped
)

func methodKind(mt reflect.Type) (rpcMethodKind, bool) {
	errType := reflect.TypeOf((*error)(nil)).Elem()
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	emptyIface := reflect.TypeOf((*interface{})(nil)).Elem()
	protoMsgType := reflect.TypeOf((*proto.Message)(nil)).Elem()

	if mt.NumOut() != 2 {
		return 0, false
	}
	if !mt.Out(1).Implements(errType) {
		return 0, false
	}

	switch mt.NumIn() {
	case 2:
		if mt.In(1) == emptyIface && mt.Out(0) == emptyIface {
			return rpcMethodArgsOnly, true
		}

	case 3:
		if !mt.In(1).Implements(ctxType) {
			return 0, false
		}

		in2Type := mt.In(2)
		out0Type := mt.Out(0)

		if in2Type == emptyIface && out0Type == emptyIface {
			return rpcMethodCtxArgs, true
		}

		if isTypedMethod(in2Type, out0Type, protoMsgType) {
			return rpcMethodTyped, true
		}
	}

	return 0, false
}

func isTypedMethod(inType, outType reflect.Type, protoMsgType reflect.Type) bool {
	if inType.Kind() != reflect.Ptr || outType.Kind() != reflect.Ptr {
		return false
	}

	if inType.Implements(protoMsgType) && outType.Implements(protoMsgType) {
		return true
	}

	return false
}

func makeMethodHandler(recv reflect.Value, m reflect.Method, kind rpcMethodKind) MethodHandler {
	return func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		switch kind {
		case rpcMethodArgsOnly:
			in := []reflect.Value{recv, reflect.ValueOf(req.Args)}
			out := m.Func.Call(in)
			if !out[1].IsNil() {
				return nil, out[1].Interface().(error)
			}
			return out[0].Interface(), nil

		case rpcMethodCtxArgs:
			in := []reflect.Value{recv, reflect.ValueOf(ctx), reflect.ValueOf(req.Args)}
			out := m.Func.Call(in)
			if !out[1].IsNil() {
				return nil, out[1].Interface().(error)
			}
			return out[0].Interface(), nil

		case rpcMethodTyped:
			return callTypedMethod(recv, m, ctx, req)

		default:
			return nil, fmt.Errorf("unsupported rpc method kind for %s", m.Name)
		}
	}
}

func callTypedMethod(recv reflect.Value, m reflect.Method, ctx context.Context, req *protocol.Request) (interface{}, error) {
	mt := m.Type
	reqType := mt.In(2).Elem()

	reqValue := reflect.New(reqType)
	reqPtr := reqValue.Interface()
	protoMsgType := reflect.TypeOf((*proto.Message)(nil)).Elem()
	isProtoMsg := reqValue.Type().Implements(protoMsgType)

	var err error
	if argsBytes, ok := req.Args.([]byte); ok {
		switch req.ArgsCodec {
		case protocol.PayloadCodecProtobuf:
			if !isProtoMsg {
				return nil, fmt.Errorf("args codec is protobuf but target type does not implement proto.Message")
			}
			err = proto.Unmarshal(argsBytes, reqPtr.(proto.Message))
		case protocol.PayloadCodecJSON:
			err = json.Unmarshal(argsBytes, reqPtr)
		case protocol.PayloadCodecRaw:
			return nil, fmt.Errorf("cannot unmarshal raw bytes into typed request")
		default:
			if isProtoMsg {
				err = proto.Unmarshal(argsBytes, reqPtr.(proto.Message))
			} else {
				err = json.Unmarshal(argsBytes, reqPtr)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("unmarshal request: %w", err)
		}
	} else if argsMap, ok := req.Args.(map[string]interface{}); ok {
		argsBytes, marshalErr := json.Marshal(argsMap)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal args: %w", marshalErr)
		}
		err = json.Unmarshal(argsBytes, reqPtr)
		if err != nil {
			return nil, fmt.Errorf("unmarshal request: %w", err)
		}
	} else if argsProto, ok := req.Args.(proto.Message); ok && isProtoMsg {
		reqBytes, marshalErr := proto.Marshal(argsProto)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal args: %w", marshalErr)
		}
		err = proto.Unmarshal(reqBytes, reqPtr.(proto.Message))
		if err != nil {
			return nil, fmt.Errorf("unmarshal request: %w", err)
		}
	} else {
		argsBytes, marshalErr := json.Marshal(req.Args)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal args: %w", marshalErr)
		}
		err = json.Unmarshal(argsBytes, reqPtr)
		if err != nil {
			return nil, fmt.Errorf("unmarshal request: %w", err)
		}
	}

	in := []reflect.Value{recv, reflect.ValueOf(ctx), reqValue}
	out := m.Func.Call(in)

	if !out[1].IsNil() {
		return nil, out[1].Interface().(error)
	}

	respValue := out[0]
	return respValue.Interface(), nil
}
