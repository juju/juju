// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

// These errors are used for storage pool operations.
const (
	// MissingPoolTypeError is used when a provider type is empty.
	MissingPoolTypeError = errors.ConstError("pool provider type is empty")
	// MissingPoolNameError is used when a name is empty.
	MissingPoolNameError = errors.ConstError("pool name is empty")
	// InvalidPoolNameError is used when a storage pool name is invalid.
	InvalidPoolNameError = errors.ConstError("pool name is not valid")
	// InvalidStorageName represents an invalid storage name.
	InvalidStorageName = errors.ConstError("invalid storage name")
	// PoolNotFoundError is used when a storage pool is not found.
	PoolNotFoundError = errors.ConstError("storage pool is not found")
	// PoolAlreadyExists is used when a storage pool already exists.
	PoolAlreadyExists = errors.ConstError("storage pool already exists")
	// ProviderTypeNotFound is used when a storage provider type is not found.
	ProviderTypeNotFound = errors.ConstError("storage provider type not found")
	// ErrNoDefaultStoragePool is returned when a storage pool is required but none is specified nor available as a default.
	ErrNoDefaultStoragePool = errors.ConstError("no storage pool specified and no default available")
)

// These errors are used for storage directives operations.
const (
	// MissingSharedStorageDirectiveError is used when a storage directive for shared storage is not provided.
	MissingSharedStorageDirectiveError = errors.ConstError("no storage directive specified")
)

const (
	// StorageNotFound describes an error that occurs when the storage being operated
	// on does not exist.
	StorageNotFound = errors.ConstError("storage not found")

	// FilesystemNotFound describes an error that occurs when the filesystem being operated
	// on does not exist.
	FilesystemNotFound = errors.ConstError("filesystem not found")

	// VolumeNotFound describes an error that occurs when the volume being operated
	// on does not exist.
	VolumeNotFound = errors.ConstError("volume not found")
)
