// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	stderrors "errors"
	"fmt"
)

// As finds the first error in err's tree that matches target, and if one is
// found, sets target to that error value and returns true. Otherwise, it
// returns false.
//
// The tree consists of err itself, followed by the errors obtained by
// repeatedly calling its Unwrap() error or Unwrap() []error method. When err
// wraps multiple errors, As examines err followed by a depth-first traversal
// of its children.
//
// An error matches target if the error's concrete value is assignable to the
// value pointed to by target, or if the error has a method As(interface{}) bool
// such that As(target) returns true. In the latter case, the As method is
// responsible for setting target.
//
// An error type might provide an As method so it can be treated as if it were a
// different error type.
//
// As panics if target is not a non-nil pointer to either a type that implements
// error, or to any interface type.
//
// As is a proxy for [pkg/errors.As] and does not alter the semantics offered by
// this function.
func As(err error, target any) bool {
	return stderrors.As(err, target)
}

// AsType is a convenience method for checking and getting an error from within
// a chain that is of type T. If no error is found of type T in the chain the
// zero value of T is returned with false. If an error in the chain implements
// As(any) bool then it's As method will be called if it's type is not of type T.

// AsType finds the first error in err's chain that is assignable to type T, and
// if a match is found, returns that error value and true. Otherwise, it returns
// T's zero value and false.
//
// AsType is equivalent to errors.As, but uses a type parameter and returns
// the target, to avoid having to define a variable before the call. For
// example, callers can replace this:
//
//	var pathError *fs.PathError
//	if errors.As(err, &pathError) {
//	    fmt.Println("Failed at path:", pathError.Path)
//	}
//
// With:
//
//	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
//	    fmt.Println("Failed at path:", pathError.Path)
//	}
func AsType[T error](err error) (T, bool) {
	var zero T
	as := As(err, &zero)
	return zero, as
}

// Errorf implements a straight through proxy for [pkg/fmt.Errorf]. The one
// change this function signature makes is that a type of [Error] is returned so
// that the resultant error can be further annotated.
func Errorf(format string, a ...any) Error {
	return link{
		newFrameTracer(fmt.Errorf(format, a...), 1),
	}
}

// HasType is a function wrapper around AsType dropping the return value T
// from AsType() making a function that can be used like:
//
//	return HasType[*MyError](err)
//
// Or
//
//	if HasType[*MyError](err) {}
func HasType[T error](err error) bool {
	_, rval := AsType[T](err)
	return rval
}

// Is reports whether any error in err's tree matches target.
//
// The tree consists of err itself, followed by the errors obtained by repeatedly
// calling its Unwrap() error or Unwrap() []error method. When err wraps multiple
// errors, Is examines err followed by a depth-first traversal of its children.
//
// An error is considered to match a target if it is equal to that target or if
// it implements a method Is(error) bool such that Is(target) returns true.
//
// An error type might provide an Is method so it can be treated as equivalent
// to an existing error. For example, if MyError defines
//
//	func (m MyError) Is(target error) bool { return target == fs.ErrExist }
//
// then Is(MyError{}, fs.ErrExist) returns true. See [syscall.Errno.Is] for
// an example in the standard library. An Is method should only shallowly
// compare err and the target and not call [Unwrap] on either.
//
// Is is a proxy for [pkg/errors.Is] and does not alter the semantics offered by
// this function.
func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

// IsOneOf reports whether any error in err's tree matches one of the target
// errors. This check works on a first match effort in that the first target
// error discovered reports back true with no further errors.
//
// If targets is empty then this func will always return false.
//
// IsOneOf is the same as writing Is(err, type1) || Is(err, type2) || Is(err, type3)
func IsOneOf(err error, targets ...error) bool {
	for _, target := range targets {
		if stderrors.Is(err, target) {
			return true
		}
	}

	return false
}

// Join returns an error that wraps the given errors.
// Any nil error values are discarded.
// Join returns nil if every value in errs is nil.
// The error formats as the concatenation of the strings obtained
// by calling the Error method of each element of errs, with a newline
// between each string.
//
// A non-nil error returned by Join implements the Unwrap() []error method.
//
// Join is a proxy for [pkg/errors.Join] with the difference being that the
// resultant error is of type [Error]
func Join(errs ...error) Error {
	if err := stderrors.Join(errs...); err != nil {
		return link{err}
	}
	return nil
}

// New returns an error that formats as the given text. Each call to New returns
// a distinct error value even if the text is identical.
//
// New is a proxy for [pkg/errors.New]. All errors returned from New are traced.
func New(text string) Error {
	return link{
		newFrameTracer(stderrors.New(text), 1),
	}
}

// Unwrap returns the result of calling the Unwrap method on err, if err's type
// contains an Unwrap method returning error. Otherwise, Unwrap returns nil.
//
// Unwrap only calls a method of the form "Unwrap() error". In particular Unwrap
// does not unwrap errors returned by [Join]
func Unwrap(err error) error {
	return stderrors.Unwrap(err)
}
