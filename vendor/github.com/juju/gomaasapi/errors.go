// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"fmt"

	"github.com/juju/errors"
)

// NoMatchError is returned when the requested action cannot be performed
// due to being unable to service due to no entities available that match the
// request.
type NoMatchError struct {
	errors.Err
}

// NewNoMatchError constructs a new NoMatchError and sets the location.
func NewNoMatchError(message string) error {
	err := &NoMatchError{Err: errors.NewErr(message)}
	err.SetLocation(1)
	return err
}

// IsNoMatchError returns true if err is a NoMatchError.
func IsNoMatchError(err error) bool {
	_, ok := errors.Cause(err).(*NoMatchError)
	return ok
}

// UnexpectedError is an error for a condition that hasn't been determined.
type UnexpectedError struct {
	errors.Err
}

// NewUnexpectedError constructs a new UnexpectedError and sets the location.
func NewUnexpectedError(err error) error {
	uerr := &UnexpectedError{Err: errors.NewErr("unexpected: %v", err)}
	uerr.SetLocation(1)
	return errors.Wrap(err, uerr)
}

// IsUnexpectedError returns true if err is an UnexpectedError.
func IsUnexpectedError(err error) bool {
	_, ok := errors.Cause(err).(*UnexpectedError)
	return ok
}

// UnsupportedVersionError refers to calls made to an unsupported api version.
type UnsupportedVersionError struct {
	errors.Err
}

// NewUnsupportedVersionError constructs a new UnsupportedVersionError and sets the location.
func NewUnsupportedVersionError(format string, args ...interface{}) error {
	err := &UnsupportedVersionError{Err: errors.NewErr(format, args...)}
	err.SetLocation(1)
	return err
}

// IsUnsupportedVersionError returns true if err is an UnsupportedVersionError.
func IsUnsupportedVersionError(err error) bool {
	_, ok := errors.Cause(err).(*UnsupportedVersionError)
	return ok
}

// WrapWithUnsupportedVersionError constructs a new
// UnsupportedVersionError wrapping the passed error.
func WrapWithUnsupportedVersionError(err error) error {
	uerr := &UnsupportedVersionError{Err: errors.NewErr("unsupported version: %v", err)}
	uerr.SetLocation(1)
	return errors.Wrap(err, uerr)
}

// DeserializationError types are returned when the returned JSON data from
// the controller doesn't match the code's expectations.
type DeserializationError struct {
	errors.Err
}

// NewDeserializationError constructs a new DeserializationError and sets the location.
func NewDeserializationError(format string, args ...interface{}) error {
	err := &DeserializationError{Err: errors.NewErr(format, args...)}
	err.SetLocation(1)
	return err
}

// WrapWithDeserializationError constructs a new DeserializationError with the
// specified message, and sets the location and returns a new error with the
// full error stack set including the error passed in.
func WrapWithDeserializationError(err error, format string, args ...interface{}) error {
	message := fmt.Sprintf(format, args...)
	// We want the deserialization error message to include the error text of the
	// previous error, but wrap it in the new type.
	derr := &DeserializationError{Err: errors.NewErr(message + ": " + err.Error())}
	derr.SetLocation(1)
	wrapped := errors.Wrap(err, derr)
	// We want the location of the wrapped error to be the caller of this function,
	// not the line above.
	if errType, ok := wrapped.(*errors.Err); ok {
		// We know it is because that is what Wrap returns.
		errType.SetLocation(1)
	}
	return wrapped
}

// IsDeserializationError returns true if err is a DeserializationError.
func IsDeserializationError(err error) bool {
	_, ok := errors.Cause(err).(*DeserializationError)
	return ok
}

// BadRequestError is returned when the requested action cannot be performed
// due to bad or incorrect parameters passed to the server.
type BadRequestError struct {
	errors.Err
}

// NewBadRequestError constructs a new BadRequestError and sets the location.
func NewBadRequestError(message string) error {
	err := &BadRequestError{Err: errors.NewErr(message)}
	err.SetLocation(1)
	return err
}

// IsBadRequestError returns true if err is a NoMatchError.
func IsBadRequestError(err error) bool {
	_, ok := errors.Cause(err).(*BadRequestError)
	return ok
}

// PermissionError is returned when the user does not have permission to do the
// requested action.
type PermissionError struct {
	errors.Err
}

// NewPermissionError constructs a new PermissionError and sets the location.
func NewPermissionError(message string) error {
	err := &PermissionError{Err: errors.NewErr(message)}
	err.SetLocation(1)
	return err
}

// IsPermissionError returns true if err is a NoMatchError.
func IsPermissionError(err error) bool {
	_, ok := errors.Cause(err).(*PermissionError)
	return ok
}

// CannotCompleteError is returned when the requested action is unable to
// complete for some server side reason.
type CannotCompleteError struct {
	errors.Err
}

// NewCannotCompleteError constructs a new CannotCompleteError and sets the location.
func NewCannotCompleteError(message string) error {
	err := &CannotCompleteError{Err: errors.NewErr(message)}
	err.SetLocation(1)
	return err
}

// IsCannotCompleteError returns true if err is a NoMatchError.
func IsCannotCompleteError(err error) bool {
	_, ok := errors.Cause(err).(*CannotCompleteError)
	return ok
}
