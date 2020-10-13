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

// InvalidIndexError creates an invalid error.
type InvalidIndexError struct {
	err error
}

func (e *InvalidIndexError) Error() string {
	return e.err.Error()
}

// ErrInvalidIndex defines a sentinel error for invalid index.
func ErrInvalidIndex() error {
	return &InvalidIndexError{
		err: errors.Errorf("invalid index"),
	}
}

// IsInvalidIndexErr returns if the error is an ErrInvalidIndex error
func IsInvalidIndexErr(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*InvalidIndexError)
	return ok
}
