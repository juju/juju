// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"errors"
	"strings"

	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/database/driver"
	internalerrors "github.com/juju/juju/internal/errors"
)

// ErrPrecheckFailure is returned when a precheck fails. This is a fatal error
// from which the transaction cannot recover.
const ErrPrecheckFailure internalerrors.ConstError = "precheck failure is fatal"

// IsErrRetryable returns true if the given error might be
// transient and the interaction can be safely retried.
// See: https://github.com/canonical/go-dqlite/issues/220
func IsErrRetryable(err error) bool {
	// If the error is always a precheck fatal error, we should not retry.
	// Exit early to avoid unnecessary checks.
	if errors.Is(err, ErrPrecheckFailure) {
		return false
	}

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
