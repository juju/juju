// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"errors"
	"strings"

	"github.com/juju/juju/database/driver"
	"github.com/mattn/go-sqlite3"
)

// IsErrRetryable returns true if the given error might be
// transient and the interaction can be safely retried.
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
	}

	return false
}
