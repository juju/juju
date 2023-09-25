// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/database"
)

const (
	// ErrDuplicate is returned when a record already exists.
	ErrDuplicate = errors.ConstError("record already exists")

	// ErrNoRecord is returned when a record does not exist.
	ErrNoRecord = errors.ConstError("record does not exist")
)

// CoerceError converts an error to a domain error.
func CoerceError(err error) error {
	cause := errors.Cause(err)
	if database.IsErrConstraintUnique(cause) {
		return errors.Wrap(err, ErrDuplicate)
	}
	if database.IsErrNotFound(cause) {
		return errors.Wrap(err, ErrNoRecord)
	}
	return errors.Trace(err)
}
