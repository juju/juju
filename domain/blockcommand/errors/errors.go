// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// NotFound describes an error that occurs when the block being operated on
	// does not exist.
	NotFound = errors.ConstError("block not found")

	// AlreadyExists describes an error that occurs when the block already
	// exists.
	AlreadyExists = errors.ConstError("block not found")
)
