// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"

	"github.com/juju/errors"
)

type publicAPINotAvailableError struct {
	registry string
}

func (e publicAPINotAvailableError) Error() string {
	return fmt.Sprintf("public registry API is not available for %q", e.registry)
}

// NewPublicAPINotAvailableError returns a publicAPINotAvailableError error.
func NewPublicAPINotAvailableError(registry string) error {
	return publicAPINotAvailableError{registry: registry}
}

type urlError interface {
	Unwrap() error
}

// IsPublicAPINotAvailableError returns true when the supplied error is
// caused by a publicAPINotAvailableError.
func IsPublicAPINotAvailableError(err error) bool {
	if err == nil {
		return false
	}
	if wrapped, ok := errors.Cause(err).(urlError); ok {
		err = wrapped.Unwrap()
	}
	_, ok := errors.Cause(err).(publicAPINotAvailableError)
	return ok
}
