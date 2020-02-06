// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"github.com/juju/errors"
)

// stopped represents an error when state is supported.
type stopped struct {
	errors.Err
}

// ErrStoppedf returns an error which satisfies IsErrStopped().
func ErrStoppedf(format string, args ...interface{}) error {
	newErr := errors.NewErr(format+" was stopped", args...)
	newErr.SetLocation(2)
	return &stopped{newErr}
}

// NewErrStopped returns an error which wraps err and satisfies IsErrStopped().
func NewErrStopped() error {
	newErr := errors.NewErr("watcher was stopped")
	newErr.SetLocation(2)
	return &stopped{newErr}
}

// IsErrStopped reports whether the error was created with ErrStoppedf() or NewErrStopped().
func IsErrStopped(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*stopped)
	return ok
}
