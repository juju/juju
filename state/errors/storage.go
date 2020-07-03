// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

type storageAttachedError struct {
	err string
}

func NewStorageAttachedError(err string) error {
	return &storageAttachedError{err: err}
}

func (s storageAttachedError) Error() string {
	return s.err
}

// IsStorageAttachedError reports whether or not the given error was caused
// by an operation on storage that should not be, but is, attached.
func IsStorageAttachedError(err error) bool {
	_, ok := errors.Cause(err).(*storageAttachedError)
	return ok
}
