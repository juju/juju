// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

// These errors are used for storage pool operations.
const (
	// MissingPoolTypeError is used when a provider type is empty.
	MissingPoolTypeError = errors.ConstError("pool provider type is empty")
	// MissingPoolNameError is used when a name is empty.
	MissingPoolNameError = errors.ConstError("pool name is empty")
	// InvalidPoolNameError is used when a storage pool name is invalid.
	InvalidPoolNameError = errors.ConstError("pool name is not valid")
	// PoolNotFoundError is used when a storage pool is not found.
	PoolNotFoundError = errors.ConstError("storage pool is not found")
	// PoolAlreadyExists is used when a storage pool already exists.
	PoolAlreadyExists = errors.ConstError("storage pool already exists")
	// ErrNoDefaultStoragePool is returned when a storage pool is required but none is specified nor available as a default.
	ErrNoDefaultStoragePool = errors.ConstError("no storage pool specified and no default available")
)

// These errors are used for storage constraints operations.
const (
	// MissingSharedStorageConstraintError is used when a storage constraint for shared storage is not provided.
	MissingSharedStorageConstraintError = errors.ConstError("no storage constraints specified")
)
