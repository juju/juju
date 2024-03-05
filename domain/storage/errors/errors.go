// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// MissingPoolTypeError is used when a provider type is empty.
	MissingPoolTypeError = errors.ConstError("pool provider type is empty")
	// MissingPoolNameError is used when a name is empty.
	MissingPoolNameError = errors.ConstError("pool name is empty")

	InvalidPoolNameError = errors.ConstError("pool name is not valid")

	PoolNotFoundError = errors.ConstError("storage pool is not found")

	PoolAlreadyExists = errors.ConstError("storage pool already exists")
)
