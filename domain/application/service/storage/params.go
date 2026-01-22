// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/domain/storage"
)

// AddUnitStorageOverride defines additional storage to add to a unit.
type AddUnitStorageOverride struct {
	// StoragePoolUUID is the storage pool UUID.
	StoragePoolUUID *storage.StoragePoolUUID

	// SizeMiB is the size of the storage instance, in MiB.
	SizeMiB *uint64

	// Count is the number of storage instances.
	Count *uint64
}
