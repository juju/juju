// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

// annotated is a wrapping error type that allows an already established error
// to be annotated with another error so that the new annotated error now
// satisfies both the original error and the annotation error with respect to
// Is() and As().
//
// annotated only implements stderrors.Unwrap() []error returning both errors
// involved in the annotation. This means that annotated errors are not
// injecting the new annotation into the error chain and calls to errors.Unwrap
// will never return the annotation error.
type annotated struct {
	error
	annotation error
}

// Error provides a way to enrich an already existing Go error and inject new
// information into the errors chain. Error and its operations are all immutable
// to the encapsulated error value.
type Error interface {
	// error is the error being wrapped.
	error

	// Add will introduce a new error into the error chain so that subsequent
	// calls to As() and Is() will be satisfied for this additional error. The
	// error being added here will not appear in the error output from Error().
	// Unwrap() does not unwrap errors that have been added.
	Add(err error) Error

	// Unwrap returns the underlying error being enriched by this interface.
	Unwrap() error
}

// link is an implementation of [Error] and represents a transparent wrapper
// around the top most error in a chain of errors. It provides a way to wrap an
// existing error and offer functions to further enrich the error chain with
// new information.
//
// link wants to be completely transparent to the error chain. All std errors
// introspection on link works on the underlying error being wrapped.
type link struct {
	// error is the underlying error being wrapped by link. We use error in
	// composition here so that the errors implementation of the interface makes
	// link conform as an error type.
	error
}

// Add will introduce a new error into the error chain so that the resultant
// error satisfies [Is] and [As]. Implements [Errors.Add].
func (l link) Add(err error) Error {
	if err == nil {
		return l
	}
	return link{annotated{l.error, err}}
}

// Unwrap implements std errors Unwrap interface by returning the underlying
// error being wrapped by link.
func (l link) Unwrap() error {
	return l.error
}

// Unwrap implements std errors Unwrap interface by returning both the
// underlying error and that of the annotation error in a slice.
func (a annotated) Unwrap() []error {
	return []error{a.error, a.annotation}
}
