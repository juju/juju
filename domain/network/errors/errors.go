// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/errors"

const (
	// ErrSpaceAlreadyExists is returned when a space already exists.
	ErrSpaceAlreadyExists = errors.ConstError("space already exists")

	// ErrSpaceNotFound is returned when a space is not found.
	ErrSpaceNotFound = errors.ConstError("space not found")
)
