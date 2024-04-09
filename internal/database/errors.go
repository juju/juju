// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"

	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

// NOTE (stickupkid): This doesn't include a generic IsErrConstraint check,
// as it is expected that you should be checking for the specific constraint
// violation. A DML statement can't possibly violate all/multiple constraints
// at the same time, hence why it's not offered.

// IsErrConstraintCheck returns true if the input error was
// returned by SQLite due to violation of a constraint check.
func IsErrConstraintCheck(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintCheck)
}

// IsErrConstraintForeignKey returns true if the input error was
// returned by SQLite due to violation of a constraint foreign key.
func IsErrConstraintForeignKey(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintForeignKey)
}

// IsErrConstraintNotNull returns true if the input error was
// returned by SQLite due to violation of a constraint not null.
func IsErrConstraintNotNull(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintNotNull)
}

// IsErrConstraintPrimaryKey returns true if the input error was
// returned by SQLite due to violation of a constraint primary key.
func IsErrConstraintPrimaryKey(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintPrimaryKey)
}

// IsErrConstraintTrigger returns true if the input error was
// returned by SQLite due to violation of a constraint trigger.
func IsErrConstraintTrigger(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintTrigger)
}

// IsErrConstraintUnique returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintUnique(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintUnique)
}

// IsErrConstraintRowID returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintRowID(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintRowID)
}

// IsErrError returns true if the input error was returned by SQLite due
// to a generic error. This is normally used when we can't determine the
// specific error type. For example:
// - no such trigger
// - no such table
// This is useful when trying to determine if the error is sqlite specific
// or internal to Juju.
func IsErrError(err error) bool {
	if err == nil || isErrLocked(err) {
		return false
	}
	return isErrCode(err, 1)
}

// IsErrNotFound returns true if the input error was returned by SQLite due
// to a missing record.
func IsErrNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows)
}

// If the error is locked, or a busy error, then we can't have a extended
// error code. In those cases, we just return early and prevent error
// unwrapping for common cases.
func isErrLocked(err error) bool {
	return errors.Is(err, sqlite3.ErrLocked) || errors.Is(err, sqlite3.ErrBusy)
}

func isErrCode(err error, code sqlite3.ErrNoExtended) bool {
	// Check if the error is a dqlite error, if so, check the code.
	var dqliteErr dqlite.Error
	if errors.As(err, &dqliteErr) {
		return dqliteErr.Code == int(code)
	}

	// TODO (stickupkid): This is a compatibility layer for sqlite3, we should
	// remove this once we are only using dqlite.
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == code
	}

	return false
}

// IsError reports if the any type passed to it is a database driver error in
// Juju. The purpose of this function is so that our domain error masking can
// assert if a specific error needs to be hidden from layers above that of the
// domain/state.
func IsError(target any) bool {
	// Check for the dqlite error type, before checking sqlite3 error type. In
	// production we should only be using dqlite, but in tests we may use
	// sqlite3 directly.
	if _, is := target.(*dqlite.Error); is {
		return true
	}
	if _, is := target.(dqlite.Error); is {
		return true
	}
	if _, is := target.(sqlite3.Error); is {
		return true
	}
	if _, is := target.(*sqlite3.Error); is {
		return true
	}
	return false
}
