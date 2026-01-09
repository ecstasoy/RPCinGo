package protocol

import (
	"fmt"
	"sync/atomic"
	"time"

	"RPCinGo/pkg/protocol/pb"
)

type PayloadCodec = pb.PayloadCodec

const (
	PayloadCodecUnknown  = pb.PayloadCodec_PAYLOAD_CODEC_UNKNOWN
	PayloadCodecRaw      = pb.PayloadCodec_PAYLOAD_CODEC_RAW
	PayloadCodecJSON     = pb.PayloadCodec_PAYLOAD_CODEC_JSON
	PayloadCodecProtobuf = pb.PayloadCodec_PAYLOAD_CODEC_PROTOBUF
)

type Request struct {
	ID             uint64       `json:"id"`
	Service        string       `json:"service"`
	Method         string       `json:"method"`
	ServiceVersion string       `json:"service_version,omitempty"`
	Args           interface{}  `json:"args"`
	Timeout        int64        `json:"timeout,omitempty"`
	IsStream       bool         `json:"is_stream,omitempty"`
	Metadata       Metadata     `json:"metadata,omitempty"`
	CreatedAt      int64        `json:"created_at,omitempty"`
	ArgsCodec      PayloadCodec `json:"args_codec,omitempty"`
}

var requestIDCounter uint64

func NewRequest(service, method string, args interface{}) *Request {
	return &Request{
		ID:        nextRequestID(),
		Service:   service,
		Method:    method,
		Args:      args,
		Metadata:  NewMetadata(),
		CreatedAt: time.Now().UnixNano(),
	}
}

func NewRequestWithVersion(service, method, version string, args interface{}) *Request {
	req := NewRequest(service, method, args)
	req.ServiceVersion = version
	return req
}

func nextRequestID() uint64 {
	return atomic.AddUint64(&requestIDCounter, 1)
}

func (r *Request) SetTimeout(timeout time.Duration) {
	r.Timeout = timeout.Milliseconds()
}

func (r *Request) GetTimeout() time.Duration {
	if r.Timeout <= 0 {
		return 5 * time.Second
	}
	return time.Duration(r.Timeout) * time.Millisecond
}

func (r *Request) SetMetadata(key, value string) {
	if r.Metadata == nil {
		r.Metadata = NewMetadata()
	}
	r.Metadata.Set(key, value)
}

func (r *Request) GetMetadata(key string) (string, bool) {
	if r.Metadata == nil {
		return "", false
	}
	return r.Metadata.Get(key)
}

func (r *Request) String() string {
	return fmt.Sprintf(
		"Request{ID=%d, Service=%s, Method=%s, Version=%s, Timeout=%dms}",
		r.ID, r.Service, r.Method, r.ServiceVersion, r.Timeout,
	)
}
