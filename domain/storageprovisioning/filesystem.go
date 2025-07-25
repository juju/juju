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

	// FilesystemID is the ID of the filesystem resource that the attachment is for.
	FilesystemID string

	// Size is the size of the filesystem in MiB.
	Size uint64
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
