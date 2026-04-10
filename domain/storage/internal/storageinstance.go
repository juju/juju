// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
)

// StorageInstanceInfo represents information about a storage instance
// in the model, including its attachments, backing filesystem and or volume,
// lifecycle state, and ownership details.
type StorageInstanceInfo struct {
	// UUID is the unique identifier for the Storage Instance.
	UUID domainstorage.StorageInstanceUUID

	// Attachments defines zero or more attachments the storage instance has
	// onto units in the model.
	Attachments []StorageInstanceInfoAttachment

	// Filesystem represents information about the underlying Filesystem
	// supporting this Storage Instance. If not set then no Filesystem exists
	// for the Storage Instance.
	Filesystem *StorageInstanceInfoFilesystem

	// Life is the current life of the Storage Instance.
	Life domainlife.Life

	// Kind is the [domainstorage.StorageKind] of the Storage Instance.
	Kind domainstorage.StorageKind

	// StorageID is the unique human readable identifier for the Storage
	// Instance.
	StorageID string

	// UnitOwner describes the unit that owns this Storage Instance. If not set
	// then no Unit owns this Storage Instance.
	UnitOwner *StorageInstanceInfoUnitOwner

	// Volume represents information about the underlying Volume supporting this
	// Storage Instance. If not set then no Volume exists for the Storage
	// Instance.
	Volume *StorageInstanceInfoVolume
}

// StorageInstanceInfoAttachment represents an attachment of a storage instance
// to a unit, including details about the filesystem mount point, volume device,
// and machine assignment if applicable.
type StorageInstanceInfoAttachment struct {
	// UUID is the unique identifier for the Storage Attachment.
	UUID domainstorage.StorageAttachmentUUID

	// Filesystem represents filesystem-specific details for this attachment,
	// including the mount point. If not set then the attachment does not have
	// filesystem-specific information.
	Filesystem *StorageInstanceInfoAttachmentFilesystem

	// Life is the current life of the Storage Attachment.
	Life domainlife.Life

	// Machine represents the machine details when the storage is attached to a
	// unit deployed on a machine. If not set then the attachment is not
	// associated with a machine.
	Machine *StorageInstanceInfoAttachmentMachine

	// Volume represents volume-specific details for this attachment, including
	// device name links. If not set then the attachment does not have
	// volume-specific information.
	Volume *StorageInstanceInfoAttachmentVolume

	// UnitName is the name of the unit to which the storage is attached.
	UnitName string

	// UnitUUID is the unique identifier of the unit to which the Storage
	// Instance is attached to.
	UnitUUID coreunit.UUID
}

// StorageInstanceInfoAttachmentFilesystem contains filesystem-specific details
// for a storage attachment, including the mount point where the filesystem is
// mounted.
type StorageInstanceInfoAttachmentFilesystem struct {
	// MountPoint is the path where the filesystem is mounted on the Unit.
	MountPoint string
}

// StorageInstanceInfoAttachmentMachine contains machine details for a storage
// attachment when the storage is attached to a unit deployed on a machine.
type StorageInstanceInfoAttachmentMachine struct {
	// UUID is the unique identifier for the machine.
	UUID coremachine.UUID

	// Name is the name of the machine.
	Name string
}

// StorageInstanceInfoAttachmentVolume contains volume-specific details for a
// storage attachment, including device name links (symlinks) to the block
// device.
type StorageInstanceInfoAttachmentVolume struct {
	// DeviceNameLinks is a list of device name symlinks
	// (e.g., /dev/disk/by-id/*) that point to the block device on the attached
	// Unit's Machine.
	DeviceNameLinks []string
}

// StorageInstanceInfoFilesystem represents filesystem information for a storage
// instance, including its UUID and current status.
type StorageInstanceInfoFilesystem struct {
	// UUID is the unique identifier for the filesystem.
	UUID domainstorage.FilesystemUUID

	// Status represents the current status of the filesystem. If not set then
	// no status information is available for the filesystem.
	Status *StorageInstanceInfoFilesystemStatus
}

// StorageInstanceInfoFilesystemStatus represents the current status of a
// filesystem, including its state, message, and last update time.
type StorageInstanceInfoFilesystemStatus struct {
	// Message is a human-readable message associated with the current status.
	Message string

	// Status is the current status type of the Filesystem.
	Status domainstatus.StorageFilesystemStatusType

	// UpdatedAt is the time when the status was last updated. If not set then
	// the status has never been updated.
	UpdatedAt *time.Time
}

// StorageInstanceInfoVolumeStatus represents the current status of a volume,
// including its state, message, and last update time.
type StorageInstanceInfoVolumeStatus struct {
	// Message is a human-readable message associated with the current status.
	Message string

	// Status is the current status type of the Volume.
	Status domainstatus.StorageVolumeStatusType

	// UpdatedAt is the time when the status was last updated. If not set then
	// the status has never been updated.
	UpdatedAt *time.Time
}

// StorageInstanceInfoVolume represents volume information for a Storage
// Instance, including its UUID and current status.
type StorageInstanceInfoVolume struct {
	// UUID is the unique identifier for the volume.
	UUID domainstorage.VolumeUUID

	// Persistent indicates if the volume will outlive the life of whatever it
	// may be attached to. Volumes would normally be persistent when the are
	// provisioned outside of a machine.
	Persistent bool

	// Status represents the current status of the volume. If not set then no
	// status information is available for the volume.
	Status *StorageInstanceInfoVolumeStatus
}

// StorageInstanceInfoUnitOwner represents the unit that owns a Storage
// Instance, including the Unit's name and UUID.
type StorageInstanceInfoUnitOwner struct {
	// UUID is the unique identifier for the unit that owns the storage instance.
	UUID coreunit.UUID

	// Name is the name of the unit that owns the storage instance.
	Name string
}

// CreateStorageInstanceWithExistingFilesystem is used to create a storage
// instance with a filesystem that is already provisioned.
type CreateStorageInstanceWithExistingFilesystem struct {
	// Name is the name of the storage.
	Name domainstorage.Name

	// RequestedSizeMiB defines the requested size of this storage instance in
	// MiB. What ends up being allocated for the storage instance will be at
	// least this value.
	RequestedSizeMiB uint64

	// StoragePoolUUID is the pool for which this storage instance is to be
	// provisioned from.
	StoragePoolUUID domainstorage.StoragePoolUUID

	// UUID is the unique identifier to associate with the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// FilesystemUUID describes the unique identifier of the filesystem to
	// create alongside the storage instance.
	FilesystemUUID domainstorage.FilesystemUUID

	// FilesystemProvisionScope describes the provision scope to assign to the
	// newly created filesystem.
	FilesystemProvisionScope domainstorage.ProvisionScope

	// FilesystemSize is the size of the provisioned filesystem.
	FilesystemSize uint64

	// FilesystemProviderID is provider's ID for the provisioned filesystem.
	FilesystemProviderID string

	// FilesystemStatusID is the value to set the storage filesystem status to.
	FilesystemStatusID int

	// FilesystemStatusMessage is the message to set the storage filesystem
	// status to.
	FilesystemStatusMessage string

	// FilesystemStatusUpdatedAt is the time at which the storage filesystem
	// status updated at should reflect.
	FilesystemStatusUpdatedAt time.Time
}

// CreateStorageInstanceWithExistingVolumeBackedFilesystem is used to create a
// storage instance with a volume backed filesystem that is already provisioned.
type CreateStorageInstanceWithExistingVolumeBackedFilesystem struct {
	CreateStorageInstanceWithExistingFilesystem

	// VolumeUUID describes the unique identifier of the volume to create
	// alongside the storage instance.
	VolumeUUID domainstorage.VolumeUUID

	// VolumeProvisionScope describes the provision scope to assign to the newly
	// created volume.
	VolumeProvisionScope domainstorage.ProvisionScope

	// VolumeSize is the size of the provisioned volume.
	VolumeSize uint64

	// VolumeProviderID is provider's ID for the provisioned volume.
	VolumeProviderID string

	// VolumeHardwareID is set by the storage provider to help matching with a
	// block device.
	VolumeHardwareID string

	// VolumeWWN is set by the storage provider to help matching with a block
	// device.
	VolumeWWN string

	// VolumePersistent is true if the volume is persistent.
	VolumePersistent bool

	// VolumeStatusID is the value to set the storage volume status to.
	VolumeStatusID int

	// VolumeStatusMessage is the message to set the storage volume status to.
	VolumeStatusMessage string

	// VolumeStatusUpdatedAt is the time at which the storage volume status
	// updated at should reflect.
	VolumeStatusUpdatedAt time.Time
}
