// +build cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

// isErrorRetryable returns true if the given error might be transient and the
// interaction can be safely retried.
func isErrorRetryable(err error) bool {
	err = errors.Cause(err)
	if err == nil {
		return false
	}

	if err, ok := err.(driver.Error); ok && err.Code == driver.ErrBusy {
		return true
	}

	if err == sqlite3.ErrLocked || err == sqlite3.ErrBusy {
		return true
	}

	if strings.Contains(err.Error(), "database is locked") {
		return true
	}

	if strings.Contains(err.Error(), "cannot start a transaction within a transaction") {
		return true
	}

	if strings.Contains(err.Error(), "bad connection") {
		return true
	}

	if strings.Contains(err.Error(), "checkpoint in progress") {
		return true
	}

	return false
}
