// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"errors"
)

var (
	ErrNotBootstrapped  = errors.New("environment is not bootstrapped")
	ErrNoInstances      = errors.New("no instances found")
	ErrPartialInstances = errors.New("only some instances were found")
)

// containersUnsupportedError indicates that the environment does not support
// creation of containers.
type containersUnsupportedError struct {
	msg string
}

func (e *containersUnsupportedError) Error() string {
	return e.msg
}

// IsContainersUnsupportedError reports whether the error
// was created by NewContainersUnsupportedError.
func IsContainersUnsupportedError(err error) bool {
	_, ok := err.(*containersUnsupportedError)
	return ok
}

// NewContainersUnsupportedError returns a new error
// which satisfies IsContainersUnsupported and reports
// the given message.
func NewContainersUnsupported(msg string) error {
	return &containersUnsupportedError{msg: msg}
}
