// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storageprovisioning"
)

// ImportStorageInstanceArgs represents data to import a storage instance
// and its owner.
type ImportStorageInstanceArgs struct {
	UUID             string
	Life             int
	PoolName         string
	RequestedSizeMiB uint64
	StorageName      string
	StorageKind      string
	StorageID        string
	UnitName         string
}

// ImportFilesystemArgs represents data to import a filesystem.
type ImportFilesystemArgs struct {
	UUID                string
	ID                  string
	Life                life.Life
	SizeInMiB           uint64
	ProviderID          string
	StorageInstanceUUID string
	Scope               storageprovisioning.ProvisionScope
}

// ImportVolumeArgs represents a volume definition used when importing
// volumes into the model.
type ImportVolumeArgs struct {
	UUID                string
	ID                  string
	LifeID              life.Life
	StorageInstanceUUID string
	StorageID           string
	Provisioned         bool
	ProvisionScopeID    storageprovisioning.ProvisionScope
	SizeMiB             uint64
	HardwareID          string
	WWN                 string
	ProviderID          string
	Persistent          bool
}
