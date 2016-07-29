// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params // import "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

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
	ErrNotFound         ErrorCode = "not found"
	ErrMetadataNotFound ErrorCode = "metadata not found"
	ErrForbidden        ErrorCode = "forbidden"
	ErrBadRequest       ErrorCode = "bad request"
	// TODO change to ErrAlreadyExists
	ErrDuplicateUpload    ErrorCode = "duplicate upload"
	ErrMultipleErrors     ErrorCode = "multiple errors"
	ErrUnauthorized       ErrorCode = "unauthorized"
	ErrMethodNotAllowed   ErrorCode = "method not allowed"
	ErrServiceUnavailable ErrorCode = "service unavailable"
	ErrEntityIdNotAllowed ErrorCode = "charm or bundle id not allowed"
	ErrInvalidEntity      ErrorCode = "invalid charm or bundle"

	// Note that these error codes sit in the same name space
	// as the bakery error codes defined in gopkg.in/macaroon-bakery.v0/httpbakery .
	// In particular, ErrBadRequest is a shared error code
	// which needs to share the message too.
)

// Error represents an error - it is returned for any response that fails.
// See https://github.com/juju/charmstore/blob/v4/docs/API.md#errors
type Error struct {
	Message string
	Code    ErrorCode
	Info    map[string]*Error `json:",omitempty"`
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

// ErrorCode holds the class of the error in
// machine readable format.
func (e *Error) ErrorCode() string {
	return e.Code.Error()
}

// ErrorInfo returns additional info on the error.
// TODO(rog) rename this so that it more accurately
// reflects its role.
func (e *Error) ErrorInfo() map[string]*Error {
	return e.Info
}

// Cause implements errgo.Causer.Cause.
func (e *Error) Cause() error {
	if e.Code != "" {
		return e.Code
	}
	return nil
}
