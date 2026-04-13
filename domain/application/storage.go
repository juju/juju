// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

// ApplicationStorageInfo defines the storage information for a given storage
// name. It does not include the name as it is expected to be mapped via [ApplicationStorage].
type ApplicationStorageInfo struct {
	// Pool is the name of the storage pool from which the storage instance
	// was provisioned.
	StoragePoolName string

	// SizeMiB is the size of the storage instance, in MiB.
	SizeMiB uint64

	// Count is the number of storage instances.
	Count uint64
}
