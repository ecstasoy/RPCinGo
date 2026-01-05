package protocol

import "fmt"

type Error struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

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

func NewError(code int32, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

func (e *Error) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}
