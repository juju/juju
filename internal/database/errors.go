// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"

	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/mattn/go-sqlite3"
)

// NOTE (stickupkid): This doesn't include a generic IsErrConstraint check,
// as it is expected that you should be checking for the specific constraint
// violation. A DML statement can't possibly violate all/multiple constraints
// at the same time, hence why it's not offered.

// IsErrConstraintCheck returns true if the input error was
// returned by SQLite due to violation of a constraint check.
func IsErrConstraintCheck(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintCheck)
}

// IsErrConstraintForeignKey returns true if the input error was
// returned by SQLite due to violation of a constraint foreign key.
func IsErrConstraintForeignKey(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintForeignKey)
}

// IsErrConstraintNotNull returns true if the input error was
// returned by SQLite due to violation of a constraint not null.
func IsErrConstraintNotNull(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintNotNull)
}

// IsErrConstraintPrimaryKey returns true if the input error was
// returned by SQLite due to violation of a constraint primary key.
func IsErrConstraintPrimaryKey(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintPrimaryKey)
}

// IsErrConstraintTrigger returns true if the input error was
// returned by SQLite due to violation of a constraint trigger.
func IsErrConstraintTrigger(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintTrigger)
}

// IsErrConstraintUnique returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintUnique(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintUnique)
}

// IsErrConstraintRowID returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintRowID(err error) bool {
	return isErrCode(err, sqlite3.ErrConstraintRowID)
}

// IsErrNotFound returns true if the input error was returned by SQLite due
// to a missing record.
func IsErrNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows)
}

// TODO (stickupkid): This is a temporary measure to help us debug an issue
// where the dqlite error code is not being set correctly. This can be removed
// once the issue is resolved.
var logger = loggo.GetLogger("juju.database")

func isErrCode(err error, code sqlite3.ErrNoExtended) bool {
	if err == nil {
		return false
	}

	var dqliteErr dqlite.Error
	if errors.As(err, &dqliteErr) {
		if dqliteErr.Code == int(code) {
			return true
		}
		// We're currently experiencing an issue where the dqlite error code
		// is not being set correctly, so we need to log the error and the error
		// code to help us debug the issue.
		// This can be removed once the issue is resolved.
		logger.Criticalf("dqlite error, checking for: %v, got: %v", code, dqliteErr.Code)
		// We know this happens in cases of constraint violations, so we can
		// log even more information to help us debug the issue.
		if code == sqlite3.ErrConstraintPrimaryKey || code == sqlite3.ErrConstraintUnique {
			logger.Criticalf("dqlite error, message: %T %T %v", err, dqliteErr, dqliteErr.Error())
		}
		return false
	}

	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		logger.Criticalf("unexpected sqlite error")
		if sqliteErr.ExtendedCode == code {
			return true
		}

		// This really, really shouldn't happen in non-test code. If it does,
		// we need to log the error and the error code to help us debug the
		// issue.
		logger.Criticalf("sqlite error, checking for: %v, got: %v", code, sqliteErr.Code)
		// We know this happens in cases of constraint violations, so we can
		// log even more information to help us debug the issue.
		if code == sqlite3.ErrConstraintPrimaryKey || code == sqlite3.ErrConstraintUnique {
			logger.Criticalf("sqlite error, message: %T %T %v", err, sqliteErr, sqliteErr.Error())
		}
		return false
	}

	// If this happens, then we need to know what the error is and what the
	// error code is.
	if code == sqlite3.ErrConstraintPrimaryKey || code == sqlite3.ErrConstraintUnique {
		logger.Criticalf("unknown error, message: %T %v", err, err.Error())
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
