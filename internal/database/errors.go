// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"
	"strings"

	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/database/drivererrors"
)

// NOTE (stickupkid): This doesn't include a generic IsErrConstraint check,
// as it is expected that you should be checking for the specific constraint
// violation. A DML statement can't possibly violate all/multiple constraints
// at the same time, hence why it's not offered.

// IsErrConstraintCheck returns true if the input error was
// returned by SQLite due to violation of a constraint check.
func IsErrConstraintCheck(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintCheck)
}

// IsErrConstraintForeignKey returns true if the input error was
// returned by SQLite due to violation of a constraint foreign key.
func IsErrConstraintForeignKey(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	if drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintTrigger) {
		// This is emitted by the FK debug triggers.
		return strings.HasPrefix(err.Error(), "Foreign Key violation")
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintForeignKey)
}

// IsErrConstraintNotNull returns true if the input error was
// returned by SQLite due to violation of a constraint not null.
func IsErrConstraintNotNull(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintNotNull)
}

// IsErrConstraintPrimaryKey returns true if the input error was
// returned by SQLite due to violation of a constraint primary key.
func IsErrConstraintPrimaryKey(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintPrimaryKey)
}

// IsErrConstraintTrigger returns true if the input error was
// returned by SQLite due to violation of a constraint trigger.
func IsErrConstraintTrigger(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintTrigger)
}

// IsErrConstraintUnique returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintUnique(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintUnique)
}

// IsErrConstraintRowID returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintRowID(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, sqlite3.ErrConstraintRowID)
}

// IsErrError returns true if the input error was returned by SQLite due
// to a generic error. This is normally used when we can't determine the
// specific error type. For example:
// - no such trigger
// - no such table
// This is useful when trying to determine if the error is sqlite specific
// or internal to Juju.
func IsExtendedErrorCode(err error) bool {
	if err == nil || drivererrors.IsErrLocked(err) {
		return false
	}
	return drivererrors.IsExtendedErrorCode(err, 1)
}

// IsErrNotFound returns true if the input error was returned by SQLite due
// to a missing record.
func IsErrNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows)
}

// IsErrRetryable returns true if the given error might be transient and the
// interaction can be safely retried.
func IsErrRetryable(err error) bool {
	return drivererrors.IsErrRetryable(err)
}
