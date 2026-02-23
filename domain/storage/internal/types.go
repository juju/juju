// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storage"
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

// RecommendedStoragePoolArg represents a recommended storage pool assignment
// for the state layer to accept.
type RecommendedStoragePoolArg struct {
	StoragePoolUUID storage.StoragePoolUUID
	StorageKind     storage.StorageKind
}
