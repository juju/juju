// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"errors"
	"strings"

	dqlite "github.com/canonical/go-dqlite/v2/driver"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/database/driver"
)

// IsErrRetryable returns true if the given error might be
// transient and the interaction can be safely retried.
// See: https://github.com/canonical/go-dqlite/v2/issues/220
func IsErrRetryable(err error) bool {
	var dErr *driver.Error
	if errors.As(err, &dErr) && dErr.Code == driver.ErrBusy {
		return true
	}

	if errors.Is(err, sqlite3.ErrLocked) || errors.Is(err, sqlite3.ErrBusy) {
		return true
	}

	// Unwrap errors one at a time.
	for ; err != nil; err = errors.Unwrap(err) {
		errStr := err.Error()

		if strings.Contains(errStr, "database is locked") {
			return true
		}

		if strings.Contains(errStr, "cannot start a transaction within a transaction") {
			return true
		}

		if strings.Contains(errStr, "bad connection") {
			return true
		}

		if strings.Contains(errStr, "checkpoint in progress") {
			return true
		}
	}

	return false
}

func isConstraintError(err error) bool {
	if err == nil || errors.Is(err, sqlite3.ErrLocked) || errors.Is(err, sqlite3.ErrBusy) {
		return false
	}
	return isErrCode(err, sqlite3.ErrConstraintCheck) ||
		isErrCode(err, sqlite3.ErrConstraintForeignKey) ||
		isErrCode(err, sqlite3.ErrConstraintNotNull) ||
		isErrCode(err, sqlite3.ErrConstraintPrimaryKey) ||
		isErrCode(err, sqlite3.ErrConstraintTrigger) ||
		isErrCode(err, sqlite3.ErrConstraintUnique) ||
		isErrCode(err, sqlite3.ErrConstraintRowID)
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
