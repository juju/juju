// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

// forcedOperation that allows accumulation of operational errors and
// can be forced.
type forcedOperation struct {
	// Force controls whether or not the removal of a unit
	// will be forced, i.e. ignore operational errors.
	Force bool

	// Errors contains errors encountered while applying this operation.
	// Generally, these are non-fatal errors that have been encountered
	// during, say, force. They may not have prevented the operation from being
	// aborted but the user might still want to know about them.
	Errors []error

	// MaxWait specifies the amount of time that each step in relation destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait time.Duration
}

// AddError adds an error to the collection of errors for this operation.
func (op *forcedOperation) AddError(one ...error) {
	op.Errors = append(op.Errors, one...)
}

// FatalError returns true if the err is not nil and Force is false.
// If the error is not nil, it's added to the slice of errors for the
// operation.
func (op *forcedOperation) FatalError(err error) bool {
	if err != nil {
		if !op.Force {
			return true
		}
		op.Errors = append(op.Errors, err)
	}
	return false
}

// LastError returns last added error for this operation.
func (op *forcedOperation) LastError() error {
	if len(op.Errors) == 0 {
		return nil
	}
	return op.Errors[len(op.Errors)-1]
}
