// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
)

// AttachStorageInstanceToUnitArg describes the arguments required for attaching
// an existing Storage Instance to a Unit in the model.
type AttachStorageInstanceToUnitArg struct {
	// CreateUnitStorageAttachmentArg holds identifiers and attachment
	// details for the Storage Instance attachment to create.
	CreateUnitStorageAttachmentArg

	// StorageInstanceAttachmentCheckArgs describes the expected attachments for
	// the Storage Instance at the time of attach. It is expected that this arg
	// is always provided.
	StorageInstanceAttachmentCheckArgs

	// StorageInstanceCharmNameSetArg describes the Charm Name to associate
	// with the storage instance when attaching. When nil no operation will be
	// performed.
	*StorageInstanceCharmNameSetArg

	// UnitStorageInstanceAttachmentCheckArgs describes the expected unit state
	// when attaching the Storage Instance.
	UnitStorageInstanceAttachmentCheckArgs
}

// StorageAttachmentComposition describes the composition of a storage
// attachment with in the model. This information is required for (re-)attaching
// existing storage in the model to a unit. To be able to properly generate
// attachments this information is required.
type StorageAttachmentComposition struct {
	// UUID is the unique id of the storage attachment.
	UUID StorageAttachmentUUID

	// StorageInstanceUUID is the unique id of the storage instance.
	StorageInstanceUUID StorageInstanceUUID

	// FilesystemAttachments is the filesystem attachment information that
	// is part of the storage attachment composition.
	FilesystemAttachment *StorageInstanceCompositionFilesystemAttachment

	// VolumeAttachments is the volume attachment information that is part
	// of the storage attachment composition.
	VolumeAttachment *StorageInstanceCompositionVolumeAttachment
}

// StorageInstanceInfo describes a storage instance and its backing storage
// details.
type StorageInstanceAttachInfo struct {
	// UUID is the unique identifier of the storage instance.
	UUID StorageInstanceUUID

	// CharmName represents the metadata name of the charm that the Storage
	// Instance is supposed to be associated with. When not set the charm
	// association has not been established as yet. This would be expected
	// for Storage Instances that have been imported externally into the model.
	CharmName *string

	// Filesystem contains backing Filesystem details when this is Filesystem
	// Storage Instance. It is nil for non-filesystem storage.
	Filesystem *StorageInstanceAttachFilesystemInfo

	// Kind is the storage kind for the Storage Instance.
	Kind StorageKind

	// Life is the current lifecycle value for the Storage Instance.
	Life domainlife.Life

	// RequestedSizeMIB is the requested size of the Storage Instance in MiB.
	RequestedSizeMIB uint64

	// StorageName is the charm storage definition name for this instance.
	// This value will always be set and available.
	StorageName string

	// Volume contains backing volume details when this Storage Instance is
	// either block storage or a Volume backed Filesystem. Nil when the Storage
	// Instance has no volume.
	Volume *StorageInstanceAttachVolumeInfo
}

// StorageInstanceAttachFilesystemInfo describes the Filesystem backing details
// for a storage instance that is having attachment information gathered about
// it.
type StorageInstanceAttachFilesystemInfo struct {
	// UUID is the unique identifier of the backing Filesystem.
	UUID FilesystemUUID

	// ProvisionScope is the provision scope of the backing Filesystem.
	ProvisionScope ProvisionScope

	// SizeMib is the provisioned size of the backing Filesystem in MiB. When
	// the Filesystem has not yet been provisioned this value will be 0.
	SizeMib uint64

	// OwningMachineUUID is the machine that owns this Filesystem, if any.
	// The Filesystem when owned by a machine can only be attached to units on
	// the same machine.
	OwningMachineUUID *coremachine.UUID
}

// StorageInstanceCompositionFilesystemAttachment describes the filesystem
// attachment information that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionFilesystemAttachment struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this filesystem attachment.
	ProviderID string

	// ProvisionScope is the provision scope of the filesystem attachment that
	// is attached to this storage instance.
	ProvisionScope ProvisionScope

	// UUID is the unique id of the filesystem attachment that is associated
	// with this storage instance.
	UUID FilesystemAttachmentUUID

	// FilesystemUUID is the unique id of the filesystem that is associated
	// with this filesystem attachment.
	FilesystemUUID FilesystemUUID
}

// StorageInstanceCompositionVolumeAttachment describes the volume information
// that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionVolumeAttachment struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this volume attachment.
	ProviderID string

	// ProvisionScope is the provision scope of the volume attachment that is
	// attached to this storage instance.
	ProvisionScope ProvisionScope

	// UUID is the unique id of the volume attachment that is associated with
	// this storage instance.
	UUID VolumeAttachmentUUID

	// VolumeUUID is the unique id of the volume that is associated with this
	// volume attachment.
	VolumeUUID VolumeUUID
}

// StorageInstanceInfoForAttach represents information about a storage instance
// in the model that is suitable for making attachment decisions. This differs
// from [StorageInstanceInfoForUnitAttach] as it does not include unit
// information. Callers are expected to supply unit context separately.
type StorageInstanceInfoForAttach struct {
	// StorageInstanceInfo holds details about the storage instance itself and
	// its backing storage.
	StorageInstanceAttachInfo

	// StorageInstanceAttachments lists existing unit attachments for the
	// storage instance, used to detect conflicts or duplicates.
	StorageInstanceAttachments []StorageInstanceUnitAttachmentID
}

// StorageInstanceInfoForUnitAttach represents the information required to make
// a decision on if a storage instance can be attached to a unit.
type StorageInstanceInfoForUnitAttach struct {
	// StorageInstanceInfoForAttach holds details about the storage instance
	// itself and its backing storage.
	StorageInstanceInfoForAttach

	// UnitAttachNamedStorageInfo holds the unit and charm storage definition metadata
	// used to validate the attach operation.
	UnitAttachNamedStorageInfo
}

// StorageInstanceUnitAttachmentID represents the identifiers involved for a
// storage instance unit attachment.
type StorageInstanceUnitAttachmentID struct {
	// UnitUUID is the UUID of the unit that is attached to the storage instance.
	UnitUUID coreunit.UUID
	// UUID is the unique identifier of the storage instance attachment.
	UUID StorageAttachmentUUID
}

// StorageInstanceAttachVolumeInfo describes the Volume backing details for a
// Storage Instance that is having attachment information gathered about it.
type StorageInstanceAttachVolumeInfo struct {
	// UUID is the unique identifier of the backing Volume.
	UUID VolumeUUID

	// ProvisionScope is the provision scope of the backing Volume.
	ProvisionScope ProvisionScope

	// SizeMiB is the provisioned size of the backing Volume in MiB. When the
	// volume has not yet been provisioned this value will be 0.
	SizeMiB uint64

	// OwningMachineUUID is the machine that owns this volume, if any. The
	// Volume when owned by a machine can only be attached to units on the
	// same machine.
	OwningMachineUUID *coremachine.UUID
}

// UnitAttachNamedStorageInfo describes information about a named storage
// definition of a Unit's charm including the unit itself. This information is
// used to validate further storage attach operations on a Unit.
type UnitAttachNamedStorageInfo struct {
	// CharmStorageDefinition holds the charm storage definition
	// used to validate unit storage operations.
	CharmStorageDefinition

	// UUID is the unique identifier of the Unit.
	UUID coreunit.UUID

	// AlreadyAttachedCount is the count of storage instances already attached
	// to this unit for the named charm storage definition.
	AlreadyAttachedCount uint32

	// CharmMetadataName is the metadata name of the charm that the unit is
	// running.
	CharmMetadataName string

	// CharmUUID is the unique identifier of the charm that the unit is running.
	CharmUUID corecharm.ID

	// Life is the current lifecycle value of the Unit.
	Life domainlife.Life

	// MachineUUID is the UUID of the machine that the Unit is running on.
	MachineUUID *coremachine.UUID

	// Name is the name of the unit (for example, mysql/0).
	Name coreunit.Name

	// NetNodeUUID is the network node uuid associated with the Unit.
	NetNodeUUID domainnetwork.NetNodeUUID
}

// UnitStorageInstanceAttachmentCheckArgs describes the expected state of a
// unit when attaching an existing storage instance.
type UnitStorageInstanceAttachmentCheckArgs struct {
	// CountLessThanEqual is the maximum storage count allowed at the time
	// the add is performed in order for the attach operation to be considered
	// successful.
	CountLessThanEqual uint32

	// CharmUUID is the expected charm UUID that the unit is using. If the
	// unit's charm UUID is different then it indicates the the unit has been
	// upgraded.
	//
	// This would result in the assumptions made about attaching an existing
	// Storage Instance to a unit invalid.
	CharmUUID corecharm.ID

	// MachineUUID is the expected machine UUID for the unit receiving the
	// attachment. If not set it is expected that the unit is not assigned to a
	// Machine.
	MachineUUID *coremachine.UUID
}
