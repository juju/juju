// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"

	"github.com/juju/errors"
)

type publicRegistryAPINotAvailableError struct {
	registry string
}

func (e publicRegistryAPINotAvailableError) Error() string {
	return fmt.Sprintf("public registry API is not available for %q", e.registry)
}

// PublicRegistryAPINotAvailableError returns a publicRegistryAPINotAvailableError error.
func PublicRegistryAPINotAvailableError(registry string) error {
	return publicRegistryAPINotAvailableError{registry: registry}
}

type urlError interface {
	Unwrap() error
}

// IsPublicRegistryAPINotAvailableError returns true when the supplied error is
// caused by a publicRegistryAPINotAvailableError.
func IsPublicRegistryAPINotAvailableError(err error) bool {
	if err == nil {
		return false
	}
	err = errors.Cause(err)
	if wrapped, ok := err.(urlError); ok {
		err = wrapped.Unwrap()
	}
	_, ok := err.(publicRegistryAPINotAvailableError)
	return ok
}
