package httprequest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	errgo "gopkg.in/errgo.v1"
)

// These constants are recognized by DefaultErrorMapper
// as mapping to the similarly named HTTP status codes.
const (
	CodeBadRequest   = "bad request"
	CodeUnauthorized = "unauthorized"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not found"
)

// DefaultErrorUnmarshaler is the default error unmarshaler
// used by Client.
var DefaultErrorUnmarshaler = ErrorUnmarshaler(new(RemoteError))

// DefaultErrorMapper is used by Server when ErrorMapper is nil. It maps
// all errors to RemoteError instances; if an error implements the
// ErrorCoder interface, the Code field will be set accordingly; some
// codes will map to specific HTTP status codes (for example, if
// ErrorCode returns CodeBadRequest, the resulting HTTP status will be
// http.StatusBadRequest).
var DefaultErrorMapper = defaultErrorMapper

func defaultErrorMapper(ctx context.Context, err error) (status int, body interface{}) {
	errorBody := errorResponseBody(err)
	switch errorBody.Code {
	case CodeBadRequest:
		status = http.StatusBadRequest
	case CodeUnauthorized:
		status = http.StatusUnauthorized
	case CodeForbidden:
		status = http.StatusForbidden
	case CodeNotFound:
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	return status, errorBody
}

// errorResponse returns an appropriate error
// response for the provided error.
func errorResponseBody(err error) *RemoteError {
	var errResp RemoteError
	cause := errgo.Cause(err)
	if cause, ok := cause.(*RemoteError); ok {
		// It's a RemoteError already; Preserve the wrapped
		// error message but copy everything else.
		errResp = *cause
		errResp.Message = err.Error()
		return &errResp
	}

	// It's not a RemoteError. Preserve as much info as we can find.
	errResp.Message = err.Error()
	if coder, ok := cause.(ErrorCoder); ok {
		errResp.Code = coder.ErrorCode()
	}
	return &errResp
}

// ErrorCoder may be implemented by an error to cause
// it to return a particular RemoteError code when
// DefaultErrorMapper is used.
type ErrorCoder interface {
	ErrorCode() string
}

// RemoteError holds the default type of a remote error
// used by Client when no custom error unmarshaler
// is set. This type is also used by DefaultErrorMapper
// to marshal errors in Server.
type RemoteError struct {
	// Message holds the error message.
	Message string

	// Code may hold a code that classifies the error.
	Code string `json:",omitempty"`

	// Info holds any other information associated with the error.
	Info *json.RawMessage `json:",omitempty"`
}

// Error implements the error interface.
func (e *RemoteError) Error() string {
	if e.Message == "" {
		return "httprequest: no error message found"
	}
	return e.Message
}

// ErrorCode implements ErrorCoder by returning e.Code.
func (e *RemoteError) ErrorCode() string {
	return e.Code
}

// Errorf returns a new RemoteError instance that uses the
// given code and formats the message with fmt.Sprintf(f, a...).
// If f is empty and there are no other arguments, code will also
// be used for the message.
func Errorf(code string, f string, a ...interface{}) *RemoteError {
	var msg string
	if f == "" && len(a) == 0 {
		msg = code
	} else {
		msg = fmt.Sprintf(f, a...)
	}
	return &RemoteError{
		Code:    code,
		Message: msg,
	}
}
