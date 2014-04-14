// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
)

// wrapper defines a way to encapsulate an error inside another error.
type wrapper struct {
	// Err is the underlying error.
	Err error

	// Msg is the annotation (prefix) of Err.
	Msg string
}

// Error implements the error interface.
func (e *wrapper) Error() string {
	if e.Msg != "" || e.Err == nil {
		if e.Err != nil {
			return fmt.Sprintf("%s: %v", e.Msg, e.Err.Error())
		}
		return e.Msg
	}
	return e.Err.Error()
}

// wrap is a helper to construct an *wrapper.
func wrap(err error, format, suffix string, args ...interface{}) *wrapper {
	return &wrapper{err, fmt.Sprintf(format+suffix, args...)}
}

// allErrors holds information for all defined errors: a satisfier
// function, wrapping and variable arguments constructors and message
// suffix. When adding new errors, add them here as well to include
// them in tests.
var allErrors = []struct {
	Satisfier       func(error) bool
	ArgsConstructor func(string, ...interface{}) error
	WrapConstructor func(error, string) error
	Suffix          string
}{
	{IsNotFound, NotFoundf, NewNotFound, " not found"},
	{IsUnauthorized, Unauthorizedf, NewUnauthorized, ""},
	{IsNotImplemented, NotImplementedf, NewNotImplemented, " not implemented"},
	{IsAlreadyExists, AlreadyExistsf, NewAlreadyExists, " already exists"},
	{IsNotSupported, NotSupportedf, NewNotSupported, " not supported"},
}

// Contextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an
// error, Contextf does nothing. All errors created with functions
// from this package are preserved when wrapping.
func Contextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		msg := fmt.Sprintf(format, args...)
		for _, errorInfo := range allErrors {
			if errorInfo.Satisfier(*err) {
				*err = errorInfo.WrapConstructor(*err, msg)
				return
			}
		}
		*err = fmt.Errorf("%s: %v", msg, *err)
	}
}

// notFound represents an error when something has not been found.
type notFound struct {
	*wrapper
}

// NotFoundf returns an error which satisfies IsNotFound().
func NotFoundf(format string, args ...interface{}) error {
	return notFound{wrap(nil, format, " not found", args...)}
}

// NewNotFound returns an error which wraps err that satisfies
// IsNotFound().
func NewNotFound(err error, msg string) error {
	return notFound{wrap(err, msg, "")}
}

// IsNotFound reports whether err was created with NotFoundf() or
// NewNotFound().
func IsNotFound(err error) bool {
	_, ok := err.(notFound)
	return ok
}

// unauthorized represents an error when an operation is unauthorized.
type unauthorized struct {
	*wrapper
}

// Unauthorizedf returns an error which satisfies IsUnauthorized().
func Unauthorizedf(format string, args ...interface{}) error {
	return unauthorized{wrap(nil, format, "", args...)}
}

// NewUnauthorized returns an error which wraps err and satisfies
// IsUnauthorized().
func NewUnauthorized(err error, msg string) error {
	return unauthorized{wrap(err, msg, "")}
}

// IsUnauthorized reports whether err was created with Unauthorizedf() or
// NewUnauthorized().
func IsUnauthorized(err error) bool {
	_, ok := err.(unauthorized)
	return ok
}

// notImplemented represents an error when something is not
// implemented.
type notImplemented struct {
	*wrapper
}

// NotImplementedf returns an error which satisfies IsNotImplemented().
func NotImplementedf(format string, args ...interface{}) error {
	return notImplemented{wrap(nil, format, " not implemented", args...)}
}

// NewNotImplemented returns an error which wraps err and satisfies
// IsNotImplemented().
func NewNotImplemented(err error, msg string) error {
	return notImplemented{wrap(err, msg, "")}
}

// IsNotImplemented reports whether err was created with
// NotImplementedf() or NewNotImplemented().
func IsNotImplemented(err error) bool {
	_, ok := err.(notImplemented)
	return ok
}

// alreadyExists represents and error when something already exists.
type alreadyExists struct {
	*wrapper
}

// AlreadyExistsf returns an error which satisfies IsAlreadyExists().
func AlreadyExistsf(format string, args ...interface{}) error {
	return alreadyExists{wrap(nil, format, " already exists", args...)}
}

// NewAlreadyExists returns an error which wraps err and satisfies
// IsAlreadyExists().
func NewAlreadyExists(err error, msg string) error {
	return alreadyExists{wrap(err, msg, "")}
}

// IsAlreadyExists reports whether the error was created with
// AlreadyExistsf() or NewAlreadyExists().
func IsAlreadyExists(err error) bool {
	_, ok := err.(alreadyExists)
	return ok
}

// notSupported represents an error when something is not supported.
type notSupported struct {
	*wrapper
}

// NotSupportedf returns an error which satisfies IsNotSupported().
func NotSupportedf(format string, args ...interface{}) error {
	return notSupported{wrap(nil, format, " not supported", args...)}
}

// NewNotSupported returns an error which wraps err and satisfies
// IsNotSupported().
func NewNotSupported(err error, msg string) error {
	return notSupported{wrap(err, msg, "")}
}

// IsNotSupported reports whether the error was created with
// NotSupportedf() or NewNotSupported().
func IsNotSupported(err error) bool {
	_, ok := err.(notSupported)
	return ok
}
