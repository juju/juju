// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
)

type invalidChecker interface {
	// Return true if not valid.
	IsInvalid() bool
}

// validationErr represents an error signifying an object with an invalid value.
type validationErr struct {
	*errors.Err
}

// IsInvalid implements invalidChecker.
func (err validationErr) IsInvalid() bool {
	return true
}

// IsInvalid returns whether the given error indicates an invalid value.
func IsInvalid(e error) bool {
	inv, ok := e.(invalidChecker)
	if !ok {
		return false
	}
	return inv.IsInvalid()
}
