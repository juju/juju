// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// CreateModelDefaultStoragePoolArg represents the arguments required for establishing
// a new model storage pool.
//
// This argument is used to see the model with the builtin and default storage
// pools that exist.
type CreateModelDefaultStoragePoolArg struct {
	Attributes map[string]string
	Name       string
	Origin     domainstorage.StoragePoolOrigin
	Type       string
	UUID       domainstorage.StoragePoolUUID
}

// SetModelStoragePoolArg represents the creation of a relationship between
// the model and a default storage pool that will be used for a given storage
// kind.
//
// This arg is for setting the desired value overwriting any previously set
// values.
type SetModelStoragePoolArg struct {
	// StorageKind is the kind that this pool will be used as a default for.
	StorageKind domainstorage.StorageKind

	// StoragePoolUUID is the UUID of the storage pool that will be used as the
	// default for [CreateModelStoragePool.StorageKind].
	StoragePoolUUID domainstorage.StoragePoolUUID
}
