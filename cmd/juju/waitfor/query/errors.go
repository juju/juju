// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"

	"github.com/juju/errors"
)

// InvalidIdentifierError creates an invalid error.
type InvalidIdentifierError struct {
	name  string
	err   error
	scope Scope
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

func (e *InvalidIdentifierError) Scope() Scope {
	return e.scope
}

// ErrInvalidIdentifier defines a sentinel error for invalid identifiers.
func ErrInvalidIdentifier(name string, scope Scope) error {
	return &InvalidIdentifierError{
		name:  name,
		scope: scope,
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

// RuntimeErrorf defines a sentinel error for runtime errors.
func RuntimeErrorf(msg string, args ...interface{}) error {
	return &RuntimeError{
		err: errors.Errorf(msg, args...),
	}
}

// IsRuntimeError returns if the error is an ErrInvalidIndex error
func IsRuntimeError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*RuntimeError)
	return ok
}

// RuntimeSyntaxError creates an invalid error.
type RuntimeSyntaxError struct {
	err     error
	Name    string
	Options []string
}

func (e *RuntimeSyntaxError) Error() string {
	return e.err.Error()
}

// ErrRuntimeSyntax defines a sentinel error for runtime syntax error.
func ErrRuntimeSyntax(msg, name string, options []string) error {
	return &RuntimeSyntaxError{
		err:     errors.Errorf(msg),
		Name:    name,
		Options: options,
	}
}

// IsRuntimeSyntaxError returns if the error is an ErrInvalidIndex error
func IsRuntimeSyntaxError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*RuntimeSyntaxError)
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
