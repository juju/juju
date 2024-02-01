// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/database"
)

const (
	// ErrDuplicate is returned when a record already exists.
	ErrDuplicate = errors.ConstError("record already exists")

	// ErrNoRecord is returned when a record does not exist.
	ErrNoRecord = errors.ConstError("record does not exist")
)

// CoerceError converts an error from a state layer into a domain specific error
// hiding the existence of any state based errors. If the error to coerce is nil
// then nil will be returned.
func CoerceError(err error) error {
	if err == nil {
		return nil
	}

	mErr := maskError{err}
	if database.IsErrConstraintUnique(err) {
		return fmt.Errorf("%w%w", ErrDuplicate, errors.Hide(mErr))
	}
	if database.IsErrNotFound(err) {
		return fmt.Errorf("%w%w", ErrNoRecord, errors.Hide(mErr))
	}
	return mErr
}

// maskError is used to mask the existence of sql related errors. It will not
// hide contents of the error message but it will stop the user from extracting
// sql error types.
//
// The design decision for this is that outside of the state layer in Juju we
// do not want people checking for the presence of sql errors in a wrapped error
// chain. It is logic where a typed error should be used instead.
type maskError struct {
	error
}

// As implements standard errors As interface. As will check if the target type
// is a sql error that is trying to be retrieved and return false.
func (e maskError) As(target any) bool {
	if database.IsError(target) {
		return false
	}
	return errors.As(e.error, target)
}

// Is implements standard errors Is interface. Is will check if the target type
// is a sql error that is trying to be retrieved and return false.
func (e maskError) Is(target error) bool {
	if database.IsError(target) {
		return false
	}
	return errors.Is(e.error, target)
}
