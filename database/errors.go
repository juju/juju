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

func isErrCode(err error, code sqlite3.ErrNoExtended) bool {
	if err == nil {
		return false
	}

	var dqliteErr dqlite.Error
	if errors.As(err, &dqliteErr) {
		return dqliteErr.Code == int(code)
	}

	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == code
}
