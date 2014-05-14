// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"

	"github.com/juju/loggo"
)

// wrapper defines a way to encapsulate an error inside another error.
type wrapper struct {
	// Err is the underlying error.
	err error

	// Msg is the annotation (prefix) of Err.
	msg string
}

// newer is implemented by error types that can add a context message
// while preserving their type.
type newer interface {
	new(msg string) error
}

// Error implements the error interface.
func (e *wrapper) Error() string {
	if e.msg != "" || e.err == nil {
		if e.err != nil {
			return fmt.Sprintf("%s: %v", e.msg, e.err.Error())
		}
		return e.msg
	}
	return e.err.Error()
}

// wrap is a helper to construct an *wrapper.
func wrap(err error, format, suffix string, args ...interface{}) wrapper {
	return wrapper{err, fmt.Sprintf(format+suffix, args...)}
}

// Contextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an
// error, Contextf does nothing. All errors created with functions
// from this package are preserved when wrapping.
func Contextf(err *error, format string, args ...interface{}) {
	if *err == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	errNewer, ok := (*err).(newer)
	if ok {
		*err = errNewer.new(msg)
		return
	}
	*err = fmt.Errorf("%s: %v", msg, *err)
}

// Maskf masks the given error (when it is not nil) with the given
// format string and arguments (like fmt.Sprintf), returning a new
// error. If *err is nil, Maskf does nothing.
func Maskf(err *error, format string, args ...interface{}) {
	if *err == nil {
		return
	}
	*err = fmt.Errorf("%s: %v", fmt.Sprintf(format, args...), *err)
}

// notFound represents an error when something has not been found.
type notFound struct {
	wrapper
}

func (e *notFound) new(msg string) error {
	return NewNotFound(e, msg)
}

// NotFoundf returns an error which satisfies IsNotFound().
func NotFoundf(format string, args ...interface{}) error {
	return &notFound{wrap(nil, format, " not found", args...)}
}

// NewNotFound returns an error which wraps err that satisfies
// IsNotFound().
func NewNotFound(err error, msg string) error {
	return &notFound{wrap(err, msg, "")}
}

// IsNotFound reports whether err was created with NotFoundf() or
// NewNotFound().
func IsNotFound(err error) bool {
	_, ok := err.(*notFound)
	return ok
}

// unauthorized represents an error when an operation is unauthorized.
type unauthorized struct {
	wrapper
}

func (e *unauthorized) new(msg string) error {
	return NewUnauthorized(e, msg)
}

// Unauthorizedf returns an error which satisfies IsUnauthorized().
func Unauthorizedf(format string, args ...interface{}) error {
	return &unauthorized{wrap(nil, format, "", args...)}
}

// NewUnauthorized returns an error which wraps err and satisfies
// IsUnauthorized().
func NewUnauthorized(err error, msg string) error {
	return &unauthorized{wrap(err, msg, "")}
}

// IsUnauthorized reports whether err was created with Unauthorizedf() or
// NewUnauthorized().
func IsUnauthorized(err error) bool {
	_, ok := err.(*unauthorized)
	return ok
}

// notImplemented represents an error when something is not
// implemented.
type notImplemented struct {
	wrapper
}

func (e *notImplemented) new(msg string) error {
	return NewNotImplemented(e, msg)
}

// NotImplementedf returns an error which satisfies IsNotImplemented().
func NotImplementedf(format string, args ...interface{}) error {
	return &notImplemented{wrap(nil, format, " not implemented", args...)}
}

// NewNotImplemented returns an error which wraps err and satisfies
// IsNotImplemented().
func NewNotImplemented(err error, msg string) error {
	return &notImplemented{wrap(err, msg, "")}
}

// IsNotImplemented reports whether err was created with
// NotImplementedf() or NewNotImplemented().
func IsNotImplemented(err error) bool {
	_, ok := err.(*notImplemented)
	return ok
}

// alreadyExists represents and error when something already exists.
type alreadyExists struct {
	wrapper
}

func (e *alreadyExists) new(msg string) error {
	return NewAlreadyExists(e, msg)
}

// AlreadyExistsf returns an error which satisfies IsAlreadyExists().
func AlreadyExistsf(format string, args ...interface{}) error {
	return &alreadyExists{wrap(nil, format, " already exists", args...)}
}

// NewAlreadyExists returns an error which wraps err and satisfies
// IsAlreadyExists().
func NewAlreadyExists(err error, msg string) error {
	return &alreadyExists{wrap(err, msg, "")}
}

// IsAlreadyExists reports whether the error was created with
// AlreadyExistsf() or NewAlreadyExists().
func IsAlreadyExists(err error) bool {
	_, ok := err.(*alreadyExists)
	return ok
}

// notSupported represents an error when something is not supported.
type notSupported struct {
	wrapper
}

func (e *notSupported) new(msg string) error {
	return NewNotSupported(e, msg)
}

// NotSupportedf returns an error which satisfies IsNotSupported().
func NotSupportedf(format string, args ...interface{}) error {
	return &notSupported{wrap(nil, format, " not supported", args...)}
}

// NewNotSupported returns an error which wraps err and satisfies
// IsNotSupported().
func NewNotSupported(err error, msg string) error {
	return &notSupported{wrap(err, msg, "")}
}

// IsNotSupported reports whether the error was created with
// NotSupportedf() or NewNotSupported().
func IsNotSupported(err error) bool {
	_, ok := err.(*notSupported)
	return ok
}

// LoggedErrorf logs the error and return an error with the same text.
func LoggedErrorf(logger loggo.Logger, format string, a ...interface{}) error {
	logger.Logf(loggo.ERROR, format, a...)
	return fmt.Errorf(format, a...)
}
