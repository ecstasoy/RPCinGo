// Kunhua Huang, 12/29/2025

package protocol

import (
	"fmt"
	"sync/atomic"
)

type Request struct {
	ID      uint64            `json:"id"`
	Service string            `json:"service"`
	Method  string            `json:"method"`
	Args    []interface{}     `json:"args"`
	Meta    map[string]string `json:"meta,omitempty"`
}

type Response struct {
	ID    uint64      `json:"id"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

func (r *Request) String() string {
	return fmt.Sprintf("Request{ID:%d, Service:%s, Method:%s, Args:%v, Meta:%v}", r.ID, r.Service, r.Method, r.Args, r.Meta)
}

func (resp *Response) String() string {
	if resp.Error != "" {
		return fmt.Sprintf("Response{ID:%d, Error:%s}", resp.ID, resp.Error)
	}
	return fmt.Sprintf("Response{ID:%d, Data:%v}", resp.ID, resp.Data)
}

func (resp *Response) IsError() bool {
	return resp.Error != ""
}

// ------------------- Assisting Functions -------------------//
var requestIDCounter uint64

func NewRequest(service, method string, args []interface{}) *Request {
	return &Request{
		ID:      nextRequestID(),
		Service: service,
		Method:  method,
		Args:    args,
		Meta:    make(map[string]string),
	}
}

func NewResponse(id uint64, data interface{}, err error) *Response {
	resp := &Response{
		ID:   id,
		Data: data,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func nextRequestID() uint64 {
	return atomic.AddUint64(&requestIDCounter, 1)
}
