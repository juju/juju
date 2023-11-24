// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"fmt"

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
		return fmt.Errorf("%w%w", maskError{error: err}, ErrDuplicate)
	}
	if database.IsErrNotFound(cause) {
		return fmt.Errorf("%w%w", maskError{error: err}, ErrNoRecord)
	}
	return errors.Trace(err)
}

// maskError is used to mask the error message, yet still allow the
// error to be identified.
type maskError struct {
	error
}

func (e maskError) Error() string {
	return ""
}
