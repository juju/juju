// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"

	"github.com/juju/errors"
)

// InvalidIdentifierError creates an invalid error.
type InvalidIdentifierError struct {
	name string
	err  error
}

func (e *InvalidIdentifierError) Error() string {
	if e.err == nil {
		return ""
	}
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

// SyntaxError creates an invalid error.
type SyntaxError struct {
	Pos          Position
	TokenType    TokenType
	Expectations []TokenType
}

func (e *SyntaxError) Error() string {
	if len(e.Expectations) == 0 {
		return fmt.Sprintf("Syntax Error: %v invalid character '%s' found", e.Pos, e.TokenType)
	}
	return fmt.Sprintf("Syntax Error: %v expected token to be %s, got %s instead", e.Pos, e.Expectations[0], e.TokenType)
}

func ErrSyntaxError(pos Position, tokenType TokenType, expectations ...TokenType) error {
	return &SyntaxError{
		Pos:          pos,
		TokenType:    tokenType,
		Expectations: expectations,
	}
}

// IsSyntaxError returns if the error is an ErrSyntaxError error
func IsSyntaxError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*SyntaxError)
	return ok
}
