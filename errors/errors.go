// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
)

// errorWrapper defines a way to encapsulate an error inside another error.
type errorWrapper struct {
	// error is the underlying error.
	Err error
	Msg string
}

// Error implements the error interface.
func (e *errorWrapper) Error() string {
	if e.Msg != "" || e.Err == nil {
		if e.Err != nil {
			return fmt.Sprintf("%s: %v", e.Msg, e.Err.Error())
		}
		return e.Msg
	}
	return e.Err.Error()
}

// notFoundError records an error when something has not been found.
type notFoundError struct {
	*errorWrapper
}

// IsNotFoundError returns true if err is a NotFoundError.
func IsNotFoundError(err error) bool {
	if _, ok := err.(notFoundError); ok {
		return true
	}
	return false
}

// NotFoundf returns a error for which IsNotFound returns
// true. The message for the error is made up from the given
// arguments formatted as with fmt.Sprintf, with the
// string " not found" appended.
func NotFoundf(format string, args ...interface{}) error {
	return notFoundError{&errorWrapper{Msg: fmt.Sprintf(format+" not found", args...)}}
}

// NewNotFoundError returns a new error from the given error and message that will
// return true from IsnotFoundError.
func NewNotFoundError(err error, msg string) error {
	return notFoundError{&errorWrapper{Err: err, Msg: msg}}
}

// unauthorizedError represents the error that an operation is unauthorized.
// Use IsUnauthorized() to determine if the error was related to authorization failure.
type unauthorizedError struct {
	*errorWrapper
}

// IsUnauthorizedError returns true if err is a UnauthorizedError.
func IsUnauthorizedError(err error) bool {
	_, ok := err.(unauthorizedError)
	return ok
}

// Unauthorizedf returns an error for which IsUnauthorizedError returns true.
// It is mainly used for testing.
func Unauthorizedf(format string, args ...interface{}) error {
	return unauthorizedError{&errorWrapper{Msg: fmt.Sprintf(format, args...)}}
}

// NewUnauthorizedError returns an error which wraps err and for which IsUnauthorized
// returns true.
func NewUnauthorizedError(err error, msg string) error {
	return unauthorizedError{&errorWrapper{Msg: msg}}
}

// notBootstrappedError indicates that the system can't be used because it hasn't been
// bootstrapped yet.
type notBootstrappedError struct {
	*errorWrapper
}

// IsNotBootstrapped returns true if err is a *NotBootstrappedError.
func IsNotBootstrapped(err error) bool {
	if _, ok := err.(notBootstrappedError); ok {
		return true
	}
	return false
}

// NewNotBootstrappedError returns an error which wraps err and for which
// IsNotBootstrapped returns true.
func NewNotBootstrappedError(err error, msg string) error {
	return notBootstrappedError{&errorWrapper{Err: err, Msg: msg}}
}
