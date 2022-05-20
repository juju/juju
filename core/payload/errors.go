// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"github.com/juju/errors"
)

var (
	// ErrAlreadyExists indicates that a payload could not be added
	// because it was already added.
	ErrAlreadyExists = errors.AlreadyExistsf("payload")

	// ErrNotFound indicates that a requested payload has not been
	// added yet.
	ErrNotFound = errors.NotFoundf("payload")
)
