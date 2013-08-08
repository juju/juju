// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
)

// NotFoundError records an error when something has not been found.
type NotFoundError struct {
	// error is the underlying error.
	error
	Msg string
}

// IsNotFoundError returns true if err is a NotFoundError.
func IsNotFoundError(err error) bool {
	if _, ok := err.(*NotFoundError); ok {
		return true
	}
	return false
}

func (e *NotFoundError) Error() string {
	if e.Msg != "" || e.error == nil {
		if e.error != nil {
			return fmt.Sprintf("%s: %v", e.Msg, e.error.Error())
		}
		return e.Msg
	}
	return e.error.Error()
}

// NotFoundf returns a error for which IsNotFound returns
// true. The message for the error is made up from the given
// arguments formatted as with fmt.Sprintf, with the
// string " not found" appended.
func NotFoundf(format string, args ...interface{}) error {
	return &NotFoundError{nil, fmt.Sprintf(format+" not found", args...)}
}

// UnauthorizedError represents the error that an operation is unauthorized.
// Use IsUnauthorized() to determine if the error was related to authorization failure.
type UnauthorizedError struct {
	error
	Msg string
}

func IsUnauthorizedError(err error) bool {
	_, ok := err.(*UnauthorizedError)
	return ok
}

func (e *UnauthorizedError) Error() string {
	if e.error != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.error.Error())
	}
	return e.Msg
}

// Unauthorizedf returns an error for which IsUnauthorizedError returns true.
// It is mainly used for testing.
func Unauthorizedf(format string, args ...interface{}) error {
	return &UnauthorizedError{nil, fmt.Sprintf(format, args...)}
}
