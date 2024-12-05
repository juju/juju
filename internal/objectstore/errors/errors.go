// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ObjectNotFound is returned when an object is not found in the store.
	ObjectNotFound = errors.ConstError("object not found")

	// ObjectAlreadyExists is returned an object is placed in the store, but
	// that object already exists.
	ObjectAlreadyExists = errors.ConstError("object already exists")
)
