package database

import (
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

// IsErrConstraintUnique returns true if the input error was
// returned by SQLite due to violation of a unique constraint.
func IsErrConstraintUnique(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
		return true
	}
	return false
}
