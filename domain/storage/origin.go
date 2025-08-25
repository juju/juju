// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// StoragePoolOrigin describes the origin source of a storage pool. The primary
// purpose of this value is to distinguish between storage pools that are
// created by users and those that exists within Juju.
type StoragePoolOrigin int

const (
	// StoragePoolOriginUser indicates that the storage pool was created by a
	// user.
	StoragePoolOriginUser = 1

	// StoragePoolOriginProviderDefault indicates that the storage pool is a
	// default offered by the storage provider.
	StoragePoolOriginProviderDefault = 3
)
