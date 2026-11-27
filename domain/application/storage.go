// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
)

// ApplicationStorageDirective describes a persisted storage directive for a
// named storage definition on an application's current charm.
type ApplicationStorageDirective struct {
	// Name is the storage name declared in the charm's metadata.
	Name corestorage.Name

	// PoolUUID is the unique identifier of the storage pool used by the
	// directive.
	PoolUUID domainstorage.StoragePoolUUID

	// PoolName is the name of the storage pool used by the directive.
	PoolName string

	// SizeMiB is the minimum size of each storage instance, in MiB.
	SizeMiB uint64

	// Count is the desired number of storage instances per unit. Zero is valid
	// when the charm storage definition allows zero instances.
	Count uint32
}
