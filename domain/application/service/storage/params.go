// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	corestorage "github.com/juju/juju/core/storage"
)

// AddUnitStorageArgs defines additional storage to add to a unit.
type AddUnitStorageArgs struct {
	// StorageName is the charm storage name.
	StorageName corestorage.Name

	// SizeMiB is the size of the storage instance, in MiB.
	SizeMiB *uint64

	// Count is the number of storage instances.
	Count *uint64
}
