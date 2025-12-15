// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

const (
	// BootstrapStoragePoolNameKey defines a const key value that is used by
	// the client when bootstrapping a controller with defined storage pools.
	// This key allows for the name of the storage pool to be defined in an
	// attribute map.
	BootstrapStoragePoolNameKey = "name"

	// BootstrapStoragePoolTypeKey defines a const key value that is used by
	// the client when bootstrapping a controller with defined storage pools.
	// This key allows for the provider type of the storage pool to be defined
	// in an attribute map.
	BootstrapStoragePoolTypeKey = "type"
)
