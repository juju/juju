// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import "github.com/pkg/errors"

// InvalidIdentifierError creates an invalid error.
type InvalidIdentifierError struct {
	name string
	err  error
}

func (e *InvalidIdentifierError) Error() string {
	return e.err.Error()
}

// Name returns the name associated with the identifier error.
func (e *InvalidIdentifierError) Name() string {
	return e.name
}

// ErrInvalidIdentifier defines a sentinel error for invalid identifiers.
func ErrInvalidIdentifier(name string) error {
	return &InvalidIdentifierError{
		name: name,
		err:  errors.Errorf("invalid identifer"),
	}
}

// IsInvalidIdentifierErr returns if the error is an ErrInvalidIdentifier error
func IsInvalidIdentifierErr(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*InvalidIdentifierError)
	return ok
}

// RuntimeError creates an invalid error.
type RuntimeError struct {
	err error
}

func (e *RuntimeError) Error() string {
	return e.err.Error()
}

// RuntimeErrorf defines a sentinel error for invalid index.
func RuntimeErrorf(msg string, args ...interface{}) error {
	return &RuntimeError{
		err: errors.Errorf("Runtime Error: "+msg, args...),
	}
}

// IsRuntimeError returns if the error is an ErrInvalidIndex error
func IsRuntimeError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*RuntimeError)
	return ok
}
