// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

// ConstError is a type for representing static const errors that are best
// composed as strings.
//
// They're great for package level errors where a package needs to indicate that
// a certain type of problem to the caller. Const errors are immutable and
// always comparable.
type ConstError string

// Error returns the constant error string encapsulated by [ConstError].
//
// Error also implements the [error] interface.
func (e ConstError) Error() string {
	return string(e)
}
