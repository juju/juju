// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
)

// errorWrapper defines a way to encapsulate an error inside another error.
type errorWrapper struct {
	// Err is the underlying error.
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

// IsNotFoundError is satisfied by errors created by this package representing
// resources that can't be found.
func IsNotFoundError(err error) bool {
	if _, ok := err.(notFoundError); ok {
		return true
	}
	return false
}

// NotFoundf returns a error which satisfies IsNotFoundError().
// The message for the error is made up from the given
// arguments formatted as with fmt.Sprintf, with the
// string " not found" appended.
func NotFoundf(format string, args ...interface{}) error {
	return notFoundError{
		&errorWrapper{
			Msg: fmt.Sprintf(format+" not found", args...),
		},
	}
}

// NewNotFoundError returns a new error wrapping err that satisfies
// IsNotFoundError().
func NewNotFoundError(err error, msg string) error {
	return notFoundError{&errorWrapper{Err: err, Msg: msg}}
}

// unauthorizedError represents the error that an operation is unauthorized.
// Use IsUnauthorized() to determine if the error was related to authorization
// failure.
type unauthorizedError struct {
	*errorWrapper
}

// IsUnauthorizedError is satisfied by errors created by this package
// representing authorization failures.
func IsUnauthorizedError(err error) bool {
	_, ok := err.(unauthorizedError)
	return ok
}

// Unauthorizedf returns an error which satisfies IsUnauthorizedError().
func Unauthorizedf(format string, args ...interface{}) error {
	return unauthorizedError{
		&errorWrapper{
			Msg: fmt.Sprintf(format, args...),
		},
	}
}

// NewUnauthorizedError returns an error which wraps err and satisfies
// IsUnauthorized().
func NewUnauthorizedError(err error, msg string) error {
	return unauthorizedError{&errorWrapper{Err: err, Msg: msg}}
}

type notImplementedError struct {
	what string
}

// NewNotImplementedError returns an error signifying that
// something is not implemented.
func NewNotImplementedError(what string) error {
	return &notImplementedError{what: what}
}

func (e *notImplementedError) Error() string {
	return e.what + " not implemented"
}

// IsNotImplementedError reports whether the error
// was created with NewNotImplementedError.
func IsNotImplementedError(err error) bool {
	_, ok := err.(*notImplementedError)
	return ok
}

type alreadyExistsError struct {
	what string
}

// NewAlreadyExistsError returns an error signifying that
// something already exists.
func NewAlreadyExistsError(what string) error {
	return &alreadyExistsError{what: what}
}

func (e *alreadyExistsError) Error() string {
	return e.what + " already exists"
}

// IsAlreadyExistsError reports whether the error
// was created with NewAlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := err.(*alreadyExistsError)
	return ok
}

type notSupportedError struct {
	what string
}

// NewNotSupportedError returns an error signifying that something is not
// supported.  For example a client API call to a server that does not support
// the action.
func NewNotSupportedError(what string) error {
	return &notSupportedError{what: what}
}

func (e *notSupportedError) Error() string {
	return e.what + " not supported"
}

// IsNotSupportedError reports whether the error
// was created with NewNotSupportedError.
func IsNotSupportedError(err error) bool {
	_, ok := err.(*notSupportedError)
	return ok
}
