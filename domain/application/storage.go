// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/domain/application/charm"
	domainstorage "github.com/juju/juju/domain/storage"
)

// StorageDirective defines a storage directive that already exists for either
// an application or unit.
type StorageDirective struct {
	// CharmMetadataName is the metadata name of the charm the directive exists for.
	CharmMetadataName string

	// Count represents the number of storage instances that should be made for
	// this directive. This value should be the desired count but not the limit.
	// For the maximum supported limit see [StorageDirective.MaxCount].
	Count uint32

	// CharmStorageType represents the storage type of the charm that the
	// directive relates to.
	CharmStorageType charm.StorageType

	// MaxCount represents the maximum number of storage instances that can be
	// made for this directive.
	MaxCount uint32

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive.
	PoolUUID domainstorage.StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

// StorageName is a type alias for storage names.
type StorageName = string

// ApplicationStorage is a map of storage names to their corresponding storage info.
type ApplicationStorage map[StorageName]ApplicationStorageInfo

// ApplicationStorageInfo defines the storage information for a given storage
// name. It does not include the name as it is expected to be mapped via [ApplicationStorage].
type ApplicationStorageInfo struct {
	// Pool is the name of the storage pool from which the storage instance
	// was provisioned.
	StoragePoolName string

	// SizeMiB is the size of the storage instance, in MiB.
	SizeMiB *uint64

	// Count is the number of storage instances.
	Count *uint64
}
