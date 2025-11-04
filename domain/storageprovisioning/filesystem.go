// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
)

// FilesystemAttachmentID is a struct that provides the IDs and names associated
// with a filesystem attachment. In this case the ID refers to the filesystem
// resource the attachment is for. As well as this the name of the machine and
// or the unit the attachment is for is also provided.
//
// As it is unclear if a filesystem attachment is for a unit or a machine either
// one of the name values will be set but not both.
type FilesystemAttachmentID struct {
	// FilesystemID is the ID of the filesystem resource that the attachment is
	// for.
	FilesystemID string

	// MachineName is the name of the machine the filesystem attachment is
	// against. Only one of [FilesystemAttachmentID.MachineName] or
	// [FilesystemAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	MachineName *coremachine.Name

	// UnitName is the name of the unit the filesystem attachment is against.
	// Only one of [FilesystemAttachmentID.MachineName] or
	// [FilesystemAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	UnitName *coreunit.Name
}

// Filesystem is a struct that provides the information about a filesystem.
type Filesystem struct {
	// BackingVolume contains information about the volume that is used to back
	// this filesystem. If this value is nil, this filesystem is not backed by
	// a volume in the model.
	BackingVolume *FilesystemBackingVolume

	// FilesystemID is the ID of the filesystem.
	FilesystemID string

	// ProviderID is the ID of the filesystem from the storage provider.
	ProviderID string

	// SizeMiB is the size of the filesystem in MiB.
	SizeMiB uint64
}

// FilesystemParams defines the set of parameters that a caller needs to know
// in order to provision a filesystem in the model.
type FilesystemParams struct {
	Attributes    map[string]string
	ID            string
	Provider      string
	ProviderID    *string
	SizeMiB       uint64
	BackingVolume *FilesystemBackingVolume
}

// FilesystemRemovalParams defines the set of parameters that a caller needs to
// know in order to de-provision a filesystem in the model.
type FilesystemRemovalParams struct {
	// Provider is the name of the provider that is provisioning this
	// filesystem.
	Provider string

	// ProviderID is the ID of the filesystem from the storage provider.
	ProviderID string

	// Obliterate is true when the provisioner should remove the filesystem from
	// existence.
	Obliterate bool
}

// FilesystemBackingVolume contains information about the volume that is used
// to back a filesystem.
type FilesystemBackingVolume struct {
	// VolumeID is the ID of the volume that the filesystem is created on.
	VolumeID string
}

// FilesystemAttachment is a struct that provides the information about a
// filesystem attachment.
type FilesystemAttachment struct {
	// FilesystemID is the ID of the filesystem resource that the attachment is for.
	FilesystemID string

	// MountPoint is the path at which the filesystem is mounted on the
	// machine. MountPoint may be empty, meaning that the filesystem is
	// not mounted yet.
	MountPoint string

	// ReadOnly indicates whether the filesystem is mounted read-only.
	ReadOnly bool
}

// FilesystemAttachmentParams defines the set of parameters that a caller needs
// to know in order to provision a filesystem attachment in the model.
type FilesystemAttachmentParams struct {
	// CharmStorageMaxCount defines the maximum number of storage instances that
	// may exist for this storage on the unit. This value is guaranteed to be a
	// non negative integer. If the charm has no defined maximum 0 will be used.
	CharmStorageCountMax int

	// CharmStorageLocation defines the recommended mount location for the
	// filesystem attachment as directed by the charm. If this filesystem
	// attachment cannot be linked to a charm storage a zero value will be used.
	CharmStorageLocation string

	// CharmStorageReadOnly indicates if the charm wants this attachment to be
	// readonly.
	CharmStorageReadOnly bool

	// MachineInstanceID is the cloud instance id given to the machine this
	// filesystem attachment is on to. If the attachment is not onto a
	// machine or no cloud instance id exists a zero value will be supplied.
	MachineInstanceID string

	// MountPoint is the path at which this filesystem attachment is mounted at.
	// Should the attachment not be mounted yet the zero value will be set.
	MountPoint string

	// Provider is the storage provider responsible for provisioning the
	// attachment.
	Provider string

	// FilesystemProviderID is the unique ID given to the filesystem from the
	// storage provider.
	FilesystemProviderID string

	// FilesystemAttachmentProviderID is the unique ID given to the filesystem
	// attachment from the storage provider.
	FilesystemAttachmentProviderID *string
}

// FilesystemTemplate represents the required information to supply a Kubernetes
// PVC template/Pod template, such that the required Filsystems for a new unit
// of the supplied application are created and mounted correctly.
type FilesystemTemplate struct {
	// Attachments describes the attachment templates for this filesystem.
	Attachments []FilesystemAttachmentTemplate

	// Attributes are a set of key value pairs that are supplied to the provider
	// or provisioner to facilitate this filesystem(s).
	Attributes map[string]string

	// Count is the number of filesystem(s) to mount for this storage.
	Count int

	// ProviderType is the name of the provider to be used to provision this
	// filesystem(s).
	ProviderType string

	// SizeMiB is the number of mebibytes to allocate for this filesystem or
	// each of these filesystems.
	SizeMiB uint64

	// StorageName is the name of the storage as defined in the charm for this
	// application.
	StorageName string
}

// FilesystemAttachmentTemplate describes an attachment that MUST be made as
// part of a [FilesystemTemplate].
type FilesystemAttachmentTemplate struct {
	// ReadOnly is true when the charm has specified the filesystem to be read
	// only mounted.
	ReadOnly bool

	// MountPoint is the location where the filesystem attachment should be
	// made.
	MountPoint string
}

// FilesystemProvisionedInfo is information set by the storage provisioner for
// filesystems it has provisioned.
type FilesystemProvisionedInfo struct {
	// ProviderID is the ID of the filesystem from the storage provider.
	ProviderID string

	// SizeMiB is the size of the filesystem in MiB.
	SizeMiB uint64
}

// FilesystemAttachmentProvisionedInfo is information set by the storage
// provisioner for filesystems attachments it has provisioned.
type FilesystemAttachmentProvisionedInfo struct {
	// MountPoint is the path where the filesystem is mounted.
	MountPoint string

	// ReadOnly is true if the filesystem is mounted read-only.
	ReadOnly bool
}
