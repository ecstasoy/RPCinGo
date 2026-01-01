package protocol

import (
	"fmt"
	"time"
)

type Response struct {
	ID         uint64      `json:"id"`
	Data       interface{} `json:"data,omitempty"`
	Error      *Error      `json:"error,omitempty"`
	Metadata   Metadata    `json:"metadata,omitempty"`
	ServerTime int64       `json:"server_time,omitempty"`
}

func NewResponse(requestID uint64) *Response {
	return &Response{
		ID:       requestID,
		Metadata: NewMetadata(),
	}
}

func NewSuccessResponse(requestID uint64, data interface{}) *Response {
	return &Response{
		ID:         requestID,
		Data:       data,
		Metadata:   NewMetadata(),
		ServerTime: time.Now().UnixNano(),
	}
}

func NewErrorResponse(requestID uint64, err *Error) *Response {
	return &Response{
		ID:         requestID,
		Error:      err,
		Metadata:   NewMetadata(),
		ServerTime: time.Now().UnixNano(),
	}
}

func (r *Response) IsSuccess() bool {
	return r.Error == nil || r.Error.Code == ErrorCodeOK
}

func (r *Response) IsError() bool {
	return r.Error != nil && r.Error.Code != ErrorCodeOK
}

func (r *Response) GetError() error {
	if r.Error == nil {
		return nil
	}
	return r.Error
}

func (r *Response) SetMetadata(key, value string) {
	if r.Metadata == nil {
		r.Metadata = NewMetadata()
	}
	r.Metadata.Set(key, value)
}

func (r *Response) GetMetadata(key string) (string, bool) {
	if r.Metadata == nil {
		return "", false
	}
	return r.Metadata.Get(key)
}

func (r *Response) String() string {
	if r.IsError() {
		return fmt.Sprintf("Response{ID=%d, Error=%s}", r.ID, r.Error)
	}
	return fmt.Sprintf("Response{ID=%d, Data=%v}", r.ID, r.Data)
}
