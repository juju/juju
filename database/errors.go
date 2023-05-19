// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"

	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

// IsErrConstraintUnique returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintUnique(err error) bool {
	if err == nil {
		return false
	}

	var dqliteErr dqlite.Error
	if errors.As(err, &dqliteErr) {
		return dqliteErr.Code == int(sqlite3.ErrConstraintUnique)
	}

	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}

// IsErrNotFound returns true if the input error was returned by SQLite due
// to a missing record.
func IsErrNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows)
}
