// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// StorageDirective defines a storage directive that already exists for either
// an application or unit. The definition lives in the storageprovisioning
// domain so that it can be shared with the relation domain, which makes the
// storage arguments for subordinate units from the storage directives of the
// subordinate application.
type StorageDirective = domainstorageprovisioning.StorageDirective

// StorageInfoForAdd represents the arguments required to
// add storage to a unit.
type StorageInfoForAdd struct {
	// CharmStorageDefinitionForValidation holds the storage definition
	// information from the Unit's charm for the purpose of validating against.
	CharmStorageDefinitionForValidation

	// AlreadyAttachedCount is the count of attached Storage Instances this Unit
	// already has for the Charm storage definition.
	AlreadyAttachedCount uint32
}

// ModelStoragePools provides the default storage pools that have been set
// within the model. If a value is nil then no default exists.
type ModelStoragePools struct {
	// BlockDevicePoolUUID provides the storage pool uuid to use for new block
	// storage.
	BlockDevicePoolUUID *domainstorage.StoragePoolUUID

	// FilesystemPoolUUID provides the storage pool uuid to use for
	// filesystem storage.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID
}

// StorageInstanceComposition describes the composition of a storage instance
// with in the model. This information is required for attaching existing
// storage in the model to a unit. To be able to properly generate attachments
// this information is required.
type StorageInstanceComposition struct {
	// Filesystem when non-nil describes the filesystem information that is part
	// of the storage composition.
	Filesystem *StorageInstanceCompositionFilesystem

	// StorageName is the name of the storage instance and can be considered to
	// be directly related to the charm storage for which it was provisioned.
	StorageName domainstorage.Name

	// UUID is the unique id of the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// Volume when non nil describes the volume information that is part of the
	// storage composition.
	Volume *StorageInstanceCompositionVolume
}

// StorageInstanceCompositionFilesystem describes the filesystem information
// that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionFilesystem struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this filesystem.
	ProviderID string

	// ProvisionScope is the provision scope of the filesystem that is
	// attached to this storage instance. This value is only considered valid
	// when [StorageInstanceComposition.FilesystemUUID] is not nil.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the filesystem that is associated with
	// this storage instance. If the value is nil then no filesystem exists.
	UUID domainstorage.FilesystemUUID
}

// StorageInstanceCompositionVolume describes the volume information that is
// part of a [StorageInstanceComposition].
type StorageInstanceCompositionVolume struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this volume.
	ProviderID string

	// ProvisionScope is the provision scope of the volume that is
	// attached to this storage instance. This value is only considered valid
	// when [StorageInstanceComposition.VolumeUUID] is not nil.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the volume that is associated with this
	// storage instance. If the value is nil then no volume exists.
	UUID domainstorage.VolumeUUID
}

// UnitStorageRefreshArgs describes the required arguments to refresh a unit
// to use a new charm with new storage.
type UnitStorageRefreshArgs struct {
	// NetNodeUUID is the net node of the unit.
	NetNodeUUID domainnetwork.NetNodeUUID

	// MachineUUID is not nil when this unit exists on a machine.
	MachineUUID *coremachine.UUID

	// CurrentCharmUUID is the uuid of the current charm the unit is using.
	CurrentCharmUUID corecharm.ID

	// RefreshCharmUUID is the uuid of the refresh charm the unit will use.
	RefreshCharmUUID corecharm.ID

	// RefreshStorageDirectives is the storage directives when the unit uses the
	// charm specified in [RefreshCharmUUID].
	RefreshStorageDirectives []StorageDirective
}
