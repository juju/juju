// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
)

// ErrorCode holds the class of an error in machine-readable format.
// It is also an error in its own right.
type ErrorCode string

func (code ErrorCode) Error() string {
	return string(code)
}

func (code ErrorCode) ErrorCode() ErrorCode {
	return code
}

const (
	ErrNotFound             ErrorCode = "not found"
	ErrForbidden            ErrorCode = "forbidden"
	ErrBadRequest           ErrorCode = "bad request"
	ErrUnauthorized         ErrorCode = "unauthorized"
	ErrAlreadyExists        ErrorCode = "already exists"
	ErrNoAdminCredsProvided ErrorCode = "no admin credentials provided"
	ErrMethodNotAllowed     ErrorCode = "method not allowed"
	ErrServiceUnavailable   ErrorCode = "service unavailable"
)

// Error represents an error - it is returned for any response that fails.
type Error struct {
	Message string    `json:"message,omitempty"`
	Code    ErrorCode `json:"code,omitempty"`
}

// NewError returns a new *Error with the given error code
// and message.
func NewError(code ErrorCode, f string, a ...interface{}) error {
	return &Error{
		Message: fmt.Sprintf(f, a...),
		Code:    code,
	}
}

// Error implements error.Error.
func (e *Error) Error() string {
	return e.Message
}

// ErrorCode holds the class of the error in machine readable format.
func (e *Error) ErrorCode() string {
	return e.Code.Error()
}

// Cause implements errgo.Causer.Cause.
func (e *Error) Cause() error {
	if e.Code != "" {
		return e.Code
	}
	return nil
}
