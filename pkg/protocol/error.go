package protocol

import "fmt"

// Error represents an application or framework error returned over RPC.
type Error struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ErrorCodeOK and related constants define the framework's well-known RPC
// status codes.
const (
	ErrorCodeOK                = 0
	ErrorCodeCanceled          = 1
	ErrorCodeUnknown           = 2
	ErrorCodeInvalidArgument   = 3
	ErrorCodeDeadlineExceeded  = 4
	ErrorCodeNotFound          = 5
	ErrorCodeAlreadyExists     = 6
	ErrorCodePermissionDenied  = 7
	ErrorCodeResourceExhausted = 8
	ErrorRequestEntityTooLarge = 9
	ErrorCodeInternal          = 13
	ErrorCodeUnavailable       = 14
)

// NewError constructs an Error with the supplied status code and message.
func NewError(code int32, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}
