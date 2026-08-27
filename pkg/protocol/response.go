package protocol

import (
	"fmt"
	"time"
)

// Response is the logical RPC response body produced by a server handler.
// Successful replies populate Data; failed replies populate Error.
type Response struct {
	ID         uint64       `json:"id"`
	Data       interface{}  `json:"data,omitempty"`
	Error      *Error       `json:"error,omitempty"`
	Metadata   Metadata     `json:"metadata,omitempty"`
	ServerTime int64        `json:"server_time,omitempty"`
	DataCodec  PayloadCodec `json:"data_codec,omitempty"`
}

// NewResponse constructs an empty Response bound to requestID.
func NewResponse(requestID uint64) *Response {
	return &Response{
		ID:       requestID,
		Metadata: NewMetadata(),
	}
}

// NewSuccessResponse constructs a successful Response and stamps the server
// timestamp.
func NewSuccessResponse(requestID uint64, data interface{}) *Response {
	return &Response{
		ID:         requestID,
		Data:       data,
		Metadata:   NewMetadata(),
		ServerTime: time.Now().UnixNano(),
	}
}

// NewErrorResponse constructs a failed Response and stamps the server
// timestamp.
func NewErrorResponse(requestID uint64, err *Error) *Response {
	return &Response{
		ID:         requestID,
		Error:      err,
		Metadata:   NewMetadata(),
		ServerTime: time.Now().UnixNano(),
	}
}

// IsSuccess reports whether the response represents a successful call.
func (r *Response) IsSuccess() bool {
	return r.Error == nil || r.Error.Code == ErrorCodeOK
}

// IsError reports whether the response carries a non-OK error payload.
func (r *Response) IsError() bool {
	return r.Error != nil && r.Error.Code != ErrorCodeOK
}

// GetError returns the embedded RPC error or nil for successful responses.
func (r *Response) GetError() error {
	if r.Error == nil {
		return nil
	}
	return r.Error
}

// SetMetadata stores a metadata key/value pair, allocating the metadata map if
// needed.
func (r *Response) SetMetadata(key, value string) {
	if r.Metadata == nil {
		r.Metadata = NewMetadata()
	}
	r.Metadata.Set(key, value)
}

// GetMetadata returns the metadata value for key and whether it was present.
func (r *Response) GetMetadata(key string) (string, bool) {
	if r.Metadata == nil {
		return "", false
	}
	return r.Metadata.Get(key)
}

// String returns a compact human-readable summary of the response.
func (r *Response) String() string {
	if r.IsError() {
		return fmt.Sprintf("Response{ID=%d, Error=%s}", r.ID, r.Error)
	}
	return fmt.Sprintf("Response{ID=%d, Data=%v}", r.ID, r.Data)
}
