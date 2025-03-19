// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"database/sql"

	"github.com/juju/juju/internal/database/drivererrors"
	"github.com/juju/juju/internal/errors"
)

// CoerceError converts all sql, sqlite and dqlite errors into an error that
// is impossible to unwrwap, thus hiding the error from errors.As and errors.Is.
// This is done to prevent checking the error type at the wrong layer. All sql
// errors should be handled at the domain layer and not above. Thus we don't
// expose couple the API client/server to the database layer.
func CoerceError(err error) error {
	if err == nil {
		return nil
	}

	// If the error is a sql error, a dqlite error or a database error, we mask
	// the error to prevent it from being unwrapped.
	if isDatabaseError(err) {
		return errors.Capture(maskError{error: err})
	}

	return err
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
	if drivererrors.IsErrorTarget(target) {
		return false
	}

	return errors.As(e.error, target)
}

// Is implements standard errors Is interface. Is will check if the target type
// is a sql error that is trying to be retrieved and return false.
func (e maskError) Is(target error) bool {
	if isDatabaseError(target) {
		return false
	}

	return errors.Is(e.error, target)
}

// isDatabaseError checks if the error is a sql, sqlite or dqlite error.
func isDatabaseError(err error) bool {
	return errors.Is(err, sql.ErrNoRows) ||
		drivererrors.IsError(err) ||
		errors.Is(err, sql.ErrTxDone) ||
		errors.Is(err, sql.ErrConnDone)
}
