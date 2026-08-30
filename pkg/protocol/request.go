package protocol

import (
	"fmt"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/protocol/pb"
)

// PayloadCodec aliases the protobuf enum used to describe how typed request
// arguments and response payloads are serialized when carried as raw bytes.
type PayloadCodec = pb.PayloadCodec

// PayloadCodecUnknown and related constants identify the serialization format
// used for typed request arguments or response payloads.
const (
	PayloadCodecUnknown  = pb.PayloadCodec_PAYLOAD_CODEC_UNKNOWN
	PayloadCodecRaw      = pb.PayloadCodec_PAYLOAD_CODEC_RAW
	PayloadCodecJSON     = pb.PayloadCodec_PAYLOAD_CODEC_JSON
	PayloadCodecProtobuf = pb.PayloadCodec_PAYLOAD_CODEC_PROTOBUF
)

// Request is the logical RPC request body exchanged between a client and a
// server before transport framing is applied.
type Request struct {
	// ID is the multiplexing key. It is owned and assigned by the transport
	// multiplexer at send time (tcp.Client.SendRequest), not by NewRequest — a
	// freshly constructed Request has ID 0. Do not rely on ID before the request
	// has been sent; logging it pre-send is misleading.
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

// NewRequest constructs a Request with initialized metadata and the current
// creation timestamp. The multiplexing ID is left unset (0); the transport
// multiplexer assigns the real per-connection ID when the request is sent.
func NewRequest(service, method string, args interface{}) *Request {
	return &Request{
		Service:   service,
		Method:    method,
		Args:      args,
		Metadata:  NewMetadata(),
		CreatedAt: time.Now().UnixNano(),
	}
}

// NewRequestWithVersion constructs a Request pinned to a specific service
// version for registries or discovery layers that support version routing.
func NewRequestWithVersion(service, method, version string, args interface{}) *Request {
	req := NewRequest(service, method, args)
	req.ServiceVersion = version
	return req
}

// SetTimeout stores the request timeout in milliseconds.
func (r *Request) SetTimeout(timeout time.Duration) {
	r.Timeout = timeout.Milliseconds()
}

// GetTimeout returns the configured request timeout.
// A zero or negative timeout falls back to 5 seconds.
func (r *Request) GetTimeout() time.Duration {
	if r.Timeout <= 0 {
		return 5 * time.Second
	}
	return time.Duration(r.Timeout) * time.Millisecond
}

// SetMetadata stores a metadata key/value pair, allocating the metadata map if
// needed.
func (r *Request) SetMetadata(key, value string) {
	if r.Metadata == nil {
		r.Metadata = NewMetadata()
	}
	r.Metadata.Set(key, value)
}

// GetMetadata returns the metadata value for key and whether it was present.
func (r *Request) GetMetadata(key string) (string, bool) {
	if r.Metadata == nil {
		return "", false
	}
	return r.Metadata.Get(key)
}

// String returns a compact human-readable summary of the request.
func (r *Request) String() string {
	return fmt.Sprintf(
		"Request{ID=%d, Service=%s, Method=%s, Version=%s, Timeout=%dms}",
		r.ID, r.Service, r.Method, r.ServiceVersion, r.Timeout,
	)
}
