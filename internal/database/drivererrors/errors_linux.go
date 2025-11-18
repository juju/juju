//go:build dqlite && linux

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package drivererrors

import (
	"errors"

	dqlite "github.com/canonical/go-dqlite/v3/driver"
	"github.com/juju/juju/internal/database/driver"
	"github.com/mattn/go-sqlite3"
)

// IsExtendedErrorCode returns true if the given error is a dqlite error with
// the given code.
// Note: ErrNo means error number, not no error.
func IsExtendedErrorCode(err error, code sqlite3.ErrNoExtended) bool {
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

// IsErrLocked returns true if the error is locked, or a busy error, then we
// can't have a extended error code. In those cases, we just return early and
// prevent error unwrapping for common cases.
func IsErrLocked(err error) bool {
	var dErr *driver.Error
	if errors.As(err, &dErr) && dErr.Code == driver.ErrBusy {
		return true
	}

	return errors.Is(err, sqlite3.ErrLocked) || errors.Is(err, sqlite3.ErrBusy)
}

// IsError reports if the any type passed to it is a database driver error in
// Juju. The purpose of this function is so that our domain error masking can
// assert if a specific error needs to be hidden from layers above that of the
// domain/state.
func IsError(err error) bool {
	// Check for the dqlite error type, before checking sqlite3 error type. In
	// production we should only be using dqlite, but in tests we may use
	// sqlite3 directly.

	// Check if the error is a dqlite error, if so, check the code.
	var dqliteErr dqlite.Error
	if errors.As(err, &dqliteErr) {
		return true
	}

	// TODO (stickupkid): This is a compatibility layer for sqlite3, we should
	// remove this once we are only using dqlite.
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr)
}

// IsErrorTarget reports if the any type passed to it is a database driver
// error.
func IsErrorTarget(target any) bool {
	if _, is := target.(*dqlite.Error); is {
		return true
	}
	if _, is := target.(dqlite.Error); is {
		return true
	}

	// TODO (stickupkid): This is a compatibility layer for sqlite3, we should
	// remove this once we are only using dqlite.
	if _, is := target.(*sqlite3.Error); is {
		return true
	}
	if _, is := target.(sqlite3.Error); is {
		return true
	}

	return false
}

// Re-export dqlite.Error as drivererrors.Error for ease of use.
type Error = dqlite.Error
