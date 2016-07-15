// This package provides an Error implementation which knows about types of error, and which has support
// for error causes.

package errors

import "fmt"

type Code string

const (
	// Public available error types.
	// These errors are provided because they are specifically required by business logic in the callers.
	UnspecifiedError    = Code("Unspecified")
	NotFoundError       = Code("NotFound")
	DuplicateValueError = Code("DuplicateValue")
	TimeoutError        = Code("Timeout")
	UnauthorisedError   = Code("Unauthorised")
	NotImplementedError = Code("NotImplemented")
)

// Error instances store an optional error cause.
type Error interface {
	error
	Cause() error
}

type gooseError struct {
	error
	errcode Code
	cause   error
}

// Type checks.
var _ Error = (*gooseError)(nil)

// Code returns the error code.
func (err *gooseError) code() Code {
	if err.errcode != UnspecifiedError {
		return err.errcode
	}
	if e, ok := err.cause.(*gooseError); ok {
		return e.code()
	}
	return UnspecifiedError
}

// Cause returns the error cause.
func (err *gooseError) Cause() error {
	return err.cause
}

// CausedBy returns true if this error or its cause are of the specified error code.
func (err *gooseError) causedBy(code Code) bool {
	if err.code() == code {
		return true
	}
	if cause, ok := err.cause.(*gooseError); ok {
		return cause.code() == code
	}
	return false
}

// Error fulfills the error interface, taking account of any caused by error.
func (err *gooseError) Error() string {
	if err.cause != nil {
		return fmt.Sprintf("%v\ncaused by: %v", err.error, err.cause)
	}
	return err.error.Error()
}

func IsNotFound(err error) bool {
	if e, ok := err.(*gooseError); ok {
		return e.causedBy(NotFoundError)
	}
	return false
}

func IsDuplicateValue(err error) bool {
	if e, ok := err.(*gooseError); ok {
		return e.causedBy(DuplicateValueError)
	}
	return false
}

func IsTimeout(err error) bool {
	if e, ok := err.(*gooseError); ok {
		return e.causedBy(TimeoutError)
	}
	return false
}

func IsUnauthorised(err error) bool {
	if e, ok := err.(*gooseError); ok {
		return e.causedBy(UnauthorisedError)
	}
	return false
}

func IsNotImplemented(err error) bool {
	if e, ok := err.(*gooseError); ok {
		return e.causedBy(NotImplementedError)
	}
	return false
}

// makeErrorf creates a new Error instance with the specified cause.
func makeErrorf(code Code, cause error, format string, args ...interface{}) Error {
	return &gooseError{
		errcode: code,
		error:   fmt.Errorf(format, args...),
		cause:   cause,
	}
}

// Newf creates a new Unspecified Error instance with the specified cause.
func Newf(cause error, format string, args ...interface{}) Error {
	return makeErrorf(UnspecifiedError, cause, format, args...)
}

// NewNotFoundf creates a new NotFound Error instance with the specified cause.
func NewNotFoundf(cause error, context interface{}, format string, args ...interface{}) Error {
	if format == "" {
		format = fmt.Sprintf("Not found: %s", context)
	}
	return makeErrorf(NotFoundError, cause, format, args...)
}

// NewDuplicateValuef creates a new DuplicateValue Error instance with the specified cause.
func NewDuplicateValuef(cause error, context interface{}, format string, args ...interface{}) Error {
	if format == "" {
		format = fmt.Sprintf("Duplicate: %s", context)
	}
	return makeErrorf(DuplicateValueError, cause, format, args...)
}

// NewTimeoutf creates a new Timeout Error instance with the specified cause.
func NewTimeoutf(cause error, context interface{}, format string, args ...interface{}) Error {
	if format == "" {
		format = fmt.Sprintf("Timeout: %s", context)
	}
	return makeErrorf(TimeoutError, cause, format, args...)
}

// NewUnauthorisedf creates a new Unauthorised Error instance with the specified cause.
func NewUnauthorisedf(cause error, context interface{}, format string, args ...interface{}) Error {
	if format == "" {
		format = fmt.Sprintf("Unauthorised: %s", context)
	}
	return makeErrorf(UnauthorisedError, cause, format, args...)
}

// NewNotImplementedf creates a new NotImplemented Error instance with the specified cause.
func NewNotImplementedf(cause error, context interface{}, format string, args ...interface{}) Error {
	if format == "" {
		format = fmt.Sprintf("Not implemented: %s", context)
	}
	return makeErrorf(NotImplementedError, cause, format, args...)
}
