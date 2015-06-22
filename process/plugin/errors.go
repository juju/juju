// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"github.com/juju/errors"
)

// validationErr represents an error signifying an object with an invalid value.
type validationErr struct {
	*errors.Err
}

// IsInvalid returns whether the given error indicates an invalid value.
func IsInvalid(e error) bool {
	_, ok := e.(validationErr)
	return ok
}
